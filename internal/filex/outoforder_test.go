package filex

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/aimuz/filedrop-cli/internal/crypto"
)

// blockingStore 让指定块一直卡住，其余块正常返回。
// 用来验证「一个块卡死不能拖住其他块」。
type blockingStore struct {
	sealed []byte
	block  int64 // 第几块要卡住
	ready  chan struct{}
}

func (s *blockingStore) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var start, end int64
	if _, err := fmt.Sscanf(r.Header.Get("range"), "bytes=%d-%d", &start, &end); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	index := start / crypto.CiphertextChunkSize
	if index == s.block {
		// 卡住，直到测试放行。
		<-s.ready
	}
	if end >= int64(len(s.sealed)) {
		end = int64(len(s.sealed)) - 1
	}
	w.WriteHeader(http.StatusPartialContent)
	w.Write(s.sealed[start : end+1])
}

// 意图：0 号块卡住时，1~N 号块应该照样下载并写到各自偏移，
// 而不是被「必须按顺序写」拖住。
func TestSlowFirstChunkDoesNotBlockOthers(t *testing.T) {
	fastBackoff(t)
	plain := make([]byte, 4*crypto.ChunkSize)
	for i := range plain {
		plain[i] = byte(i * 11)
	}
	sealed, fileID, keys, meta := buildFile(t, plain)

	store := &blockingStore{sealed: sealed, block: 0, ready: make(chan struct{})}
	server := httptest.NewServer(store)
	defer server.Close()

	out := newSink(t)
	done := make(chan error, 1)
	go func() {
		done <- fetch(context.Background(), nil, fileID, server.URL, sealed, meta, keys, out, 4)
	}()

	// 等到后面的块真正落盘（0 号还卡着）。
	// 注意不能只看服务器「收到请求」：收到之后还要解密、WriteAt。
	waitFor(t, 10*time.Second, func() bool {
		file, err := os.Open(out.path)
		if err != nil {
			return false
		}
		defer file.Close()
		probe := make([]byte, 16)
		for index := int64(1); index < int64(meta.ChunkCount); index++ {
			offset := index * crypto.ChunkSize
			if _, err := file.ReadAt(probe, offset); err != nil {
				return false
			}
			if string(probe) != string(plain[offset:offset+16]) {
				return false
			}
		}
		return true
	})

	// 此时 1~3 号块必须已经写进文件了，而整体还没结束。
	// 用 ReadAt 看具体偏移：WriteAt 会先把文件撑大，Size() 说明不了内容。
	file, err := os.Open(out.path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	for index := 1; index < meta.ChunkCount; index++ {
		// 只读该块开头几个字节，够判断是否已写入即可。
		probe := make([]byte, 16)
		offset := int64(index) * crypto.ChunkSize
		if _, err := file.ReadAt(probe, offset); err != nil {
			t.Fatalf("第 %d 块应该已经落盘: %v", index, err)
		}
		if want := plain[offset : offset+16]; string(probe) != string(want) {
			t.Fatalf("第 %d 块落盘的内容不对", index)
		}
	}
	// 0 号块还卡着，它那块应该还是空洞（全零）。
	zero := make([]byte, 16)
	if _, err := file.ReadAt(zero, 0); err != nil {
		t.Fatal(err)
	}
	if string(zero) == string(plain[:16]) {
		t.Fatal("0 号块不该已经写入")
	}

	close(store.ready)
	if err := <-done; err != nil {
		t.Fatalf("放行后应正常完成: %v", err)
	}
	if err := out.close(); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(out.path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(plain) {
		t.Fatal("最终内容与原文不一致")
	}
}

func waitFor(t *testing.T, limit time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("等待超时")
}
