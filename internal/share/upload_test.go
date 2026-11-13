package share

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/aimuz/filedrop-cli/internal/api"
	"github.com/aimuz/filedrop-cli/internal/crypto"
)

// partStore 是对象存储的替身：按 URL 里的分片号收下密文，并回一个 ETag。
type partStore struct {
	mu    sync.Mutex
	parts map[int][]byte
}

func (s *partStore) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	number, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/part/"))
	if err != nil || r.Method != http.MethodPut {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	s.mu.Lock()
	if s.parts == nil {
		s.parts = make(map[int][]byte)
	}
	s.parts[number] = body
	s.mu.Unlock()
	w.Header().Set("ETag", fmt.Sprintf(`"etag-%d"`, number))
}

func (s *partStore) get(number int) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.parts[number]
}

func newUpload(t *testing.T, plain []byte) (content *Content, keys *crypto.Keys, fileID string, meta crypto.Meta) {
	t.Helper()
	idBytes, err := crypto.RandomBytes(crypto.FileIDLength)
	if err != nil {
		t.Fatal(err)
	}
	fileID = crypto.Base64URL(idBytes)
	masterKey, err := crypto.RandomBytes(crypto.MasterKeyLength)
	if err != nil {
		t.Fatal(err)
	}
	keys, err = crypto.DeriveKeys(masterKey, fileID)
	if err != nil {
		t.Fatal(err)
	}
	size := int64(len(plain))
	content = &Content{
		Name: "blob.bin",
		Base: "https://filedrop.app",
		size: size,
		open: func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(plain)), nil },
	}
	meta = crypto.NewMeta(content.Name, size, "application/octet-stream", crypto.ExpectedChunkCount(size))
	return content, keys, fileID, meta
}

// verifyParts 按分片号解密，拼回明文并核对每个分片的 ETag。
func verifyParts(t *testing.T, store *partStore, parts []api.CompletedPart, keys *crypto.Keys, fileID string, meta crypto.Meta, plain []byte) {
	t.Helper()
	if len(parts) != meta.ChunkCount {
		t.Fatalf("完成的分片数 %d，期望 %d", len(parts), meta.ChunkCount)
	}
	var rebuilt []byte
	for index, part := range parts {
		if part.PartNumber != index+1 {
			t.Fatalf("第 %d 个分片号是 %d", index, part.PartNumber)
		}
		if want := fmt.Sprintf("etag-%d", part.PartNumber); part.ETag != want {
			t.Fatalf("分片 %d 的 ETag=%q，期望 %q", part.PartNumber, part.ETag, want)
		}
		sealed := store.get(part.PartNumber)
		if sealed == nil {
			t.Fatalf("分片 %d 没有上传", part.PartNumber)
		}
		last := index == meta.ChunkCount-1
		opened, err := keys.OpenChunk(fileID, index, last, sealed)
		if err != nil {
			t.Fatalf("第 %d 块解密失败: %v", index, err)
		}
		if last {
			opened = opened[:meta.Size-int64(index)*crypto.ChunkSize]
		}
		rebuilt = append(rebuilt, opened...)
	}
	if !bytes.Equal(rebuilt, plain) {
		t.Fatalf("拼回的内容与原文不一致（%d 字节 vs %d 字节）", len(rebuilt), len(plain))
	}
}

// 上传是并发的，但分片号、ETag 与内容必须一一对应。
func TestUploadPartsPreservesChunks(t *testing.T) {
	plain := make([]byte, 2*crypto.ChunkSize+12345)
	for i := range plain {
		plain[i] = byte(i*31 + 7)
	}
	content, keys, fileID, meta := newUpload(t, plain)

	store := &partStore{}
	server := httptest.NewServer(store)
	defer server.Close()

	urls := make(map[int]string, meta.ChunkCount)
	for number := 1; number <= meta.ChunkCount; number++ {
		urls[number] = fmt.Sprintf("%s/part/%d", server.URL, number)
	}

	parts, err := uploadParts(context.Background(), nil, fileID, content, keys, urls, nil)
	if err != nil {
		t.Fatal(err)
	}
	verifyParts(t, store, parts, keys, fileID, meta, plain)
}

// 预签名地址只给了一部分时，缺的应按窗口批量补签，而不是每块都问一次。
func TestUploadPartsSignsMissingPartsInBatch(t *testing.T) {
	plain := make([]byte, crypto.ChunkSize+99)
	content, keys, fileID, meta := newUpload(t, plain)

	store := &partStore{}
	storeServer := httptest.NewServer(store)
	defer storeServer.Close()

	var (
		mu      sync.Mutex
		batches [][]int
	)
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			PartNumbers []int `json:"partNumbers"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		batches = append(batches, body.PartNumbers)
		mu.Unlock()
		urls := make([]api.PresignedPart, 0, len(body.PartNumbers))
		for _, number := range body.PartNumbers {
			urls = append(urls, api.PresignedPart{
				PartNumber: number,
				URL:        fmt.Sprintf("%s/part/%d", storeServer.URL, number),
			})
		}
		json.NewEncoder(w).Encode(map[string]any{"partUrls": urls})
	}))
	defer apiServer.Close()

	parts, err := uploadParts(context.Background(), api.New(apiServer.URL, "token"), fileID, content, keys, map[int]string{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	verifyParts(t, store, parts, keys, fileID, meta, plain)

	mu.Lock()
	defer mu.Unlock()
	if len(batches) != 1 {
		t.Fatalf("两块应一次批量补签，实际请求 %d 次: %v", len(batches), batches)
	}
	if len(batches[0]) != 2 || batches[0][0] != 1 || batches[0][1] != 2 {
		t.Fatalf("补签的分片号不对: %v", batches[0])
	}
}
