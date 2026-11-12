package filex

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aimuz/filedrop-cli/internal/crypto"
)

// 写标准输出时必须严格按块序，并行取块会让到达顺序是乱的。
// 乱序输出会直接把用户的管道数据写坏。
func TestPipelineWritesInOrder(t *testing.T) {
	plain := make([]byte, 3*crypto.ChunkSize+4096)
	for i := range plain {
		plain[i] = byte(i*7 + 3)
	}
	sealed, fileID, keys, meta := buildFile(t, plain)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var start, end int64
		if _, err := fmt.Sscanf(r.Header.Get("range"), "bytes=%d-%d", &start, &end); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if end >= int64(len(sealed)) {
			end = int64(len(sealed)) - 1
		}
		w.WriteHeader(http.StatusPartialContent)
		w.Write(sealed[start : end+1])
	}))
	defer server.Close()

	var stdout bytes.Buffer
	out := &sink{stdout: true, path: "-", writer: &stdout}
	if err := fetch(t.Context(), nil, fileID, server.URL, sealed, meta, keys, out, 4); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stdout.Bytes(), plain) {
		t.Fatalf("标准输出的内容或顺序不对（得到 %d 字节，期望 %d 字节）", stdout.Len(), len(plain))
	}
}
