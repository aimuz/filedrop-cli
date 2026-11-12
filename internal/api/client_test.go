package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBearerAndErrorMapping(t *testing.T) {
	var seenAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("authorization")
		switch r.URL.Path {
		case "/api/account":
			json.NewEncoder(w).Encode(AccountInfo{UsedBytes: 10, QuotaBytes: 100, FileCount: 1})
		case "/api/account/files":
			json.NewEncoder(w).Encode(map[string]any{"files": []AccountFile{
				{FileID: "fQdGQ7ALzoyhsnN01te_GA", EncryptedSize: 2048, ExpiresAt: 1},
			}})
		case "/api/auth/session":
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":{"code":"unauthorized","message":"not signed in"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := New(server.URL+"/", "token-123")
	usage, err := client.Account(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if usage.QuotaBytes != 100 {
		t.Fatalf("配额解析不对: %+v", usage)
	}
	if seenAuth != "Bearer token-123" {
		t.Fatalf("没有带 Bearer 头: %q", seenAuth)
	}
	if client.BaseURL != server.URL {
		t.Fatalf("BaseURL 应去掉末尾斜杠: %s", client.BaseURL)
	}

	files, err := client.ListFiles(context.Background())
	if err != nil || len(files) != 1 || files[0].FileID != "fQdGQ7ALzoyhsnN01te_GA" {
		t.Fatalf("文件列表解析不对: %+v %v", files, err)
	}

	_, err = client.Session(context.Background())
	apiErr, ok := errors.AsType[*Error](err)
	if !ok {
		t.Fatalf("应返回结构化错误，得到 %v", err)
	}
	if apiErr.Status != 401 || apiErr.Code != "unauthorized" || !Unauthorized(err) {
		t.Fatalf("错误映射不对: %+v", apiErr)
	}
}
