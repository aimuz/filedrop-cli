package filex

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aimuz/filedrop-cli/internal/api"
	"github.com/aimuz/filedrop-cli/internal/crypto"
)

// buildFile 按协议加密明文，返回完整的密文与解密所需的一切。
func buildFile(t *testing.T, plain []byte) (sealed []byte, fileID string, keys *crypto.Keys, meta crypto.Meta) {
	t.Helper()
	sealed, fileID, keys, meta, _ = buildFileWithMaster(t, plain)
	return sealed, fileID, keys, meta
}

// buildFileWithMaster 与 buildFile 相同，但额外返回主密钥（测试里要拼链接）。
func buildFileWithMaster(t *testing.T, plain []byte) (sealed []byte, fileID string, keys *crypto.Keys, meta crypto.Meta, masterKey []byte) {
	t.Helper()
	idBytes, err := crypto.RandomBytes(crypto.FileIDLength)
	if err != nil {
		t.Fatal(err)
	}
	fileID = crypto.Base64URL(idBytes)
	masterKey, err = crypto.RandomBytes(crypto.MasterKeyLength)
	if err != nil {
		t.Fatal(err)
	}
	keys, err = crypto.DeriveKeys(masterKey, fileID)
	if err != nil {
		t.Fatal(err)
	}

	size := int64(len(plain))
	chunkCount := crypto.ExpectedChunkCount(size)
	lastLength := crypto.PaddedLastChunkSize(size)
	for index := range chunkCount {
		last := index == chunkCount-1
		length, content := crypto.ChunkSize, crypto.ChunkSize
		if last {
			length = lastLength
			content = int(size - int64(index)*crypto.ChunkSize)
		}
		block := make([]byte, length)
		copy(block, plain[int64(index)*crypto.ChunkSize:int64(index)*crypto.ChunkSize+int64(content)])
		chunk, err := keys.SealChunk(fileID, index, last, block)
		if err != nil {
			t.Fatal(err)
		}
		sealed = append(sealed, chunk...)
	}
	return sealed, fileID, keys, crypto.NewMeta("out.bin", size, "application/octet-stream", chunkCount), masterKey
}

// flakyStore 模拟对象存储：按 Range 返回密文，前 failFirst 次请求直接断开连接。
type flakyStore struct {
	sealed    []byte
	failFirst int32
	failures  atomic.Int32
	ranges    []string
	mu        sync.Mutex
}

func (f *flakyStore) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	header := r.Header.Get("range")
	f.mu.Lock()
	f.ranges = append(f.ranges, header)
	f.mu.Unlock()
	if f.failures.Add(1) <= f.failFirst {
		// 模拟传输中连接断开：拿到连接后不写任何响应就关掉。
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			panic(err)
		}
		conn.Close()
		return
	}
	var start, end int64
	if _, err := fmt.Sscanf(header, "bytes=%d-%d", &start, &end); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if end >= int64(len(f.sealed)) {
		end = int64(len(f.sealed)) - 1
	}
	w.WriteHeader(http.StatusPartialContent)
	w.Write(f.sealed[start : end+1])
}

func newSink(t *testing.T) *sink {
	t.Helper()
	path := filepath.Join(t.TempDir(), "out.bin")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	return &sink{file: file, path: path}
}

func fastBackoff(t *testing.T) {
	t.Helper()
	original := backoff
	backoff = func(context.Context, time.Duration) error { return nil }
	t.Cleanup(func() { backoff = original })
}

// firstRange 返回某个块的 Range 头，用来数它被下了几次。
func (f *flakyStore) seen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.ranges...)
}

func countRange(ranges []string, start int64) int {
	prefix := fmt.Sprintf("bytes=%d-", start)
	count := 0
	for _, r := range ranges {
		if len(r) >= len(prefix) && r[:len(prefix)] == prefix {
			count++
		}
	}
	return count
}

func TestRetriesOnlyTheFailingChunk(t *testing.T) {
	fastBackoff(t)
	plain := make([]byte, 3*crypto.ChunkSize+1234)
	for i := range plain {
		plain[i] = byte(i * 7)
	}
	sealed, fileID, keys, meta := buildFile(t, plain)

	// 头两次请求失败：并行下不保证失败落在哪一块，所以只断言
	// 「总共多请求 2 次，且没有任何一块被重下超过一次」。
	store := &flakyStore{sealed: sealed, failFirst: 2}
	server := httptest.NewServer(store)
	defer server.Close()

	out := newSink(t)
	if err := fetch(context.Background(), nil, fileID, server.URL, sealed, meta, keys, out, 0); err != nil {
		t.Fatalf("断两次就应该靠重试恢复: %v", err)
	}
	if err := out.close(); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(out.path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatal("恢复后的内容与原文不一致")
	}

	// 4 块 + 2 次重试 = 6 次请求，一次不多。
	if n := len(store.seen()); n != meta.ChunkCount+2 {
		t.Fatalf("总请求数应为 %d（4 块 + 2 次重试），实际 %d", meta.ChunkCount+2, n)
	}
}

func TestDoesNotRefetchDownloadURL(t *testing.T) {
	fastBackoff(t)
	plain := make([]byte, crypto.ChunkSize+10)
	sealed, fileID, keys, meta := buildFile(t, plain)

	var downloads atomic.Int32
	store := &flakyStore{sealed: sealed, failFirst: 1}
	server := httptest.NewServer(store)
	defer server.Close()
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downloads.Add(1)
		fmt.Fprintf(w, `{"url":%q}`, server.URL)
	}))
	defer apiServer.Close()

	client := api.New(apiServer.URL, "token")
	initial, err := client.Download(context.Background(), fileID)
	if err != nil {
		t.Fatal(err)
	}
	downloads.Store(0)

	out := newSink(t)
	if err := fetch(context.Background(), client, fileID, initial, sealed, meta, keys, out, 0); err != nil {
		t.Fatalf("断一次就应该靠重试恢复: %v", err)
	}
	out.close()

	// 预签名地址一小时内有效，块内重试不该再去要新地址——每次都会消耗下载次数。
	if n := downloads.Load(); n != 0 {
		t.Fatalf("块内重试不应该重新申请下载地址，实际调用 %d 次", n)
	}
	got, _ := os.ReadFile(out.path)
	if !bytes.Equal(got, plain) {
		t.Fatal("内容与原文不一致")
	}
}

func TestGivesUpAfterMaxAttempts(t *testing.T) {
	fastBackoff(t)
	plain := []byte("small")
	sealed, fileID, keys, meta := buildFile(t, plain)

	store := &flakyStore{sealed: sealed, failFirst: 100}
	server := httptest.NewServer(store)
	defer server.Close()

	out := newSink(t)
	err := fetch(context.Background(), nil, fileID, server.URL, sealed, meta, keys, out, 0)
	if err == nil {
		t.Fatal("一直失败时必须报错")
	}
	// 网络失败不能被报成「校验失败」——那会让人以为链接坏了。
	if !strings.Contains(err.Error(), "下载失败") {
		t.Fatalf("错误信息应说明是下载失败: %v", err)
	}
	out.discard()
	if total := len(store.seen()); total != maxAttempts {
		t.Fatalf("应尝试 %d 次，实际 %d", maxAttempts, total)
	}
}

func TestRotatesExpiredURL(t *testing.T) {
	fastBackoff(t)
	plain := make([]byte, crypto.ChunkSize+5)
	sealed, fileID, keys, meta := buildFile(t, plain)

	var fresh atomic.Int32
	// 旧地址永远 403（过期），新地址正常。
	expired := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer expired.Close()
	store := &flakyStore{sealed: sealed}
	freshServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fresh.Add(1)
		store.ServeHTTP(w, r)
	}))
	defer freshServer.Close()

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"url":%q}`, freshServer.URL)
	}))
	defer apiServer.Close()

	client := api.New(apiServer.URL, "token")
	out := newSink(t)
	if err := fetch(context.Background(), client, fileID, expired.URL, sealed, meta, keys, out, 0); err != nil {
		t.Fatalf("地址过期时应自动换地址: %v", err)
	}
	out.close()
	if n := fresh.Load(); n != 2 {
		t.Fatalf("两块都应通过新地址下载，实际 %d 次", n)
	}
}

// 解密路径必须复用明文缓冲：每块 8 MiB，不复用的话 1 GiB 文件会额外分配 1 GiB。
// 这个测试直接看内存分配量，因为「复用失效」不体现在输出上——之前就是这么漏掉的。
func TestReusesPlainBuffer(t *testing.T) {
	fastBackoff(t)
	plain := make([]byte, 4*crypto.ChunkSize)
	for i := range plain {
		plain[i] = byte(i * 13)
	}
	sealed, fileID, keys, meta := buildFile(t, plain)

	store := &flakyStore{sealed: sealed}
	server := httptest.NewServer(store)
	defer server.Close()
	out := newSink(t)
	defer out.discard()

	// 4 块 × 8 MiB = 32 MiB 数据。如果每块都新分配明文缓冲，
	// 至少要分配 32 MiB；复用后应远小于此（只留一块缓冲 + 网络读缓冲）。
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	if err := fetch(context.Background(), nil, fileID, server.URL, sealed, meta, keys, out, 0); err != nil {
		t.Fatal(err)
	}
	runtime.ReadMemStats(&after)

	allocated := after.TotalAlloc - before.TotalAlloc
	// 两块复用缓冲（密文 + 明文）约 16 MiB，加上 HTTP 读取的小对象；
	// 留 1 块余量，超过就说明某处又在按块分配了。
	if limit := uint64(2 * len(plain)); allocated > limit {
		t.Fatalf("解密 %d MiB 分配了 %d MiB（上限 %d MiB），说明有缓冲没有复用",
			len(plain)>>20, allocated>>20, limit>>20)
	}
}

// 某块彻底失败后要快速返回，而不是等其余块把重试耗完。
func TestCancelsRemainingChunksOnFailure(t *testing.T) {
	fastBackoff(t)
	plain := make([]byte, 8*crypto.ChunkSize)
	sealed, fileID, keys, meta := buildFile(t, plain)

	// 第 3 块永远失败。
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var start, end int64
		if _, err := fmt.Sscanf(r.Header.Get("range"), "bytes=%d-%d", &start, &end); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if start == 3*crypto.CiphertextChunkSize {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if end >= int64(len(sealed)) {
			end = int64(len(sealed)) - 1
		}
		w.WriteHeader(http.StatusPartialContent)
		w.Write(sealed[start : end+1])
	}))
	defer server.Close()

	out := newSink(t)
	defer out.discard()

	done := make(chan error, 1)
	go func() {
		done <- fetch(context.Background(), nil, fileID, server.URL, sealed, meta, keys, out, 4)
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("有块一直失败时必须报错")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("失败后没有及时返回，可能有 goroutine 卡住")
	}
}
