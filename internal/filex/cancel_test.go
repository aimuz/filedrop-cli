package filex

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aimuz/filedrop-cli/internal/api"
	"github.com/aimuz/filedrop-cli/internal/crypto"
)

// 取消（Ctrl-C）要能立刻停掉正在传的块，而不是等这一块慢慢传完。
// 同时未完成的文件必须删掉，不能留下半截内容。
func TestSaveCancelStopsDownload(t *testing.T) {
	plain := make([]byte, crypto.ChunkSize+4096)
	sealed, fileID, keys, meta, masterKey := buildFileWithMaster(t, plain)
	metaCipher, err := keys.SealMeta(fileID, meta)
	if err != nil {
		t.Fatal(err)
	}

	started := make(chan struct{}, 1)
	store := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case started <- struct{}{}:
		default:
		}
		// 一直占着连接，直到客户端取消——模拟慢速链路。
		<-r.Context().Done()
	}))
	defer store.Close()

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/files/"+fileID:
			json.NewEncoder(w).Encode(api.FileInfo{
				Status:        "active",
				EncryptedSize: int64(len(sealed)),
				ChunkCount:    meta.ChunkCount,
				MetaCipher:    metaCipher,
			})
		case strings.HasSuffix(r.URL.Path, "/download"):
			json.NewEncoder(w).Encode(map[string]string{"url": store.URL})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer apiServer.Close()

	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		target := fileID + "#" + crypto.Base64URL(masterKey)
		_, err := Save(ctx, api.New(apiServer.URL, "token"), target, SaveOptions{OutDir: dir})
		done <- err
	}()

	select {
	case <-started:
	case <-time.After(10 * time.Second):
		cancel()
		t.Fatal("下载请求没有发出去")
	}
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("取消后应返回 context.Canceled，得到 %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("取消后没有及时返回，可能卡在已发出的请求上")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("取消后不应留下未完成的文件: %v", entries)
	}
}
