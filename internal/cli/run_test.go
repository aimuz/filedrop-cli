package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aimuz/filedrop-cli/internal/config"
	"github.com/aimuz/filedrop-cli/internal/i18n"
)

func testRun(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	a := New(Options{
		Context: context.Background(),
		In:      strings.NewReader(""),
		Out:     out,
		Err:     errOut,
		Version: "dev",
	})
	err = a.Run(args)
	return out.String(), errOut.String(), err
}

// setupConfig 在临时目录写一份指向测试服务器的配置，返回配置文件路径。
func setupConfig(t *testing.T, apiURL, token string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("FILEDROP_CONFIG", path)
	t.Setenv("FILEDROP_API_URL", apiURL)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Token = token
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunVersionAndHelp(t *testing.T) {
	out, _, err := testRun(t, "version")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "filedrop dev" {
		t.Fatalf("version 输出不对: %q", out)
	}

	out, _, err = testRun(t, "help", "save")
	if err != errHelp {
		t.Fatalf("help save 应返回 errHelp，得到 %v", err)
	}
	if !strings.Contains(out, i18n.C().CLI.OptionsHeader) || !strings.Contains(out, "--out") {
		t.Fatalf("help save 输出不对: %q", out)
	}

	out, _, err = testRun(t, "help")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, i18n.C().CLI.Usage) {
		t.Fatal("help 无参数时应打印总览")
	}
}

func TestRunNoArgsPrintsUsage(t *testing.T) {
	out, _, err := testRun(t)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, i18n.C().CLI.Usage) {
		t.Fatal("无参数时应打印总览")
	}
}

func TestRunUnknownCommand(t *testing.T) {
	if _, _, err := testRun(t, "nope"); err == nil {
		t.Fatal("未知命令必须报错")
	} else if _, ok := err.(*usageError); !ok {
		t.Fatalf("应返回 usageError，得到 %T", err)
	}
}

func TestRunList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/account/files" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"files": []map[string]any{{
			"fileId":        "fQdGQ7ALzoyhsnN01te_GA",
			"encryptedSize": 2048,
			"expiresAt":     time.Now().Add(2 * time.Hour).UnixMilli(),
		}}})
	}))
	defer server.Close()
	setupConfig(t, server.URL, "token-123")

	out, errOut, err := testRun(t, "list")
	if err != nil {
		t.Fatal(err)
	}
	if errOut != "" {
		t.Fatalf("不该写 stderr: %q", errOut)
	}
	if !strings.Contains(out, "fQdGQ7ALzoyhsnN01te_GA") || !strings.Contains(out, i18n.C().CLI.ColID) {
		t.Fatalf("list 输出不对: %q", out)
	}
}

func TestRunQuota(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/account" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"usedBytes": 1024, "quotaBytes": 2048, "fileCount": 2})
	}))
	defer server.Close()
	setupConfig(t, server.URL, "token-123")

	out, _, err := testRun(t, "quota")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "1.0 KiB / 2.0 KiB") || !strings.Contains(out, "2") {
		t.Fatalf("quota 输出不对: %q", out)
	}
}

// 一次删多个时，单个失败不应中断其余删除。
func TestRunRmContinuesAfterFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/good"):
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/bad"):
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{
				"code": "not_found", "message": "gone",
			}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	setupConfig(t, server.URL, "token-123")

	out, errOut, err := testRun(t, "rm", "good", "bad")
	if err == nil {
		t.Fatal("有删除失败时应返回错误")
	}
	if !strings.Contains(out, "good") {
		t.Fatalf("成功的 ID 应打印出来: %q", out)
	}
	if !strings.Contains(errOut, "bad") {
		t.Fatalf("失败的 ID 应写到 stderr: %q", errOut)
	}
}

func TestRunLogoutClearsToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/auth/logout" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	path := setupConfig(t, server.URL, "token-123")

	out, _, err := testRun(t, "logout")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, i18n.C().CLI.LoggedOut) {
		t.Fatalf("logout 输出不对: %q", out)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "token-123") {
		t.Fatalf("令牌应已清除: %s", raw)
	}
}

func TestRunShareRequiresLogin(t *testing.T) {
	setupConfig(t, "http://127.0.0.1:1", "")

	_, _, err := testRun(t, "share", "x")
	if err == nil || !strings.Contains(err.Error(), i18n.C().CLI.ShareNeedsLogin) {
		t.Fatalf("未登录分享应报错，得到 %v", err)
	}
}
