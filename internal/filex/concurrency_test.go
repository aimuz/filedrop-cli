package filex

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aimuz/filedrop-cli/internal/crypto"
)

// 每个请求固定延迟，模拟高 RTT 链路：并行下载的价值主要在这里体现。
type slowStore struct {
	sealed  []byte
	latency time.Duration
}

func (s *slowStore) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	time.Sleep(s.latency)
	var start, end int64
	_, err := fmt.Sscanf(r.Header.Get("range"), "bytes=%d-%d", &start, &end)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if end >= int64(len(s.sealed)) {
		end = int64(len(s.sealed)) - 1
	}
	w.WriteHeader(http.StatusPartialContent)
	w.Write(s.sealed[start : end+1])
}

// 并行下载的价值在 RTT 高的链路上：每块的固定延迟不再串起来累加。
// 这里用 30ms 的人造延迟，验证 4 路确实明显快于串行。
func TestConcurrencySpeedup(t *testing.T) {
	if testing.Short() {
		t.Skip("耗时较长，-short 下跳过")
	}
	const chunks = 16
	plain := make([]byte, chunks*crypto.ChunkSize)
	sealed, fileID, keys, meta := buildFile(t, plain)
	server := httptest.NewServer(&slowStore{sealed: sealed, latency: 30 * time.Millisecond})
	defer server.Close()

	elapsed := make(map[int]time.Duration)
	for _, workers := range []int{1, 4} {
		start := time.Now()
		path := filepath.Join(t.TempDir(), "out")
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		out := &sink{file: f, path: path}
		err = fetch(context.Background(), nil, fileID, server.URL, sealed, meta, keys, out, workers)
		if err != nil {
			t.Fatal(err)
		}
		out.close()
		elapsed[workers] = time.Since(start)
	}

	single, parallel := elapsed[1], elapsed[4]
	t.Logf("串行 %v，4 路并行 %v（%.1f×）",
		single.Round(time.Millisecond), parallel.Round(time.Millisecond), float64(single)/float64(parallel))
	if parallel*2 > single {
		t.Fatalf("4 路并行应至少快一倍，实际串行 %v / 并行 %v", single, parallel)
	}
}
