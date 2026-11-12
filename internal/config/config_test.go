package config

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultsAndSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.json")
	t.Setenv("FILEDROP_CONFIG", path)
	t.Setenv(EnvAPIURL, "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIURL() != DefaultAPIURL || cfg.Token != "" {
		t.Fatalf("默认值不对: %+v", cfg)
	}

	cfg.Token = "secret-token"
	cfg.Account = "dingli"
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("配置文件权限应为 0600，实际 %o", info.Mode().Perm())
	}

	reloaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Token != "secret-token" || reloaded.Account != "dingli" {
		t.Fatalf("重新读取不对: %+v", reloaded)
	}
}

func TestEnvOverridesAPIURL(t *testing.T) {
	t.Setenv("FILEDROP_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	t.Setenv(EnvAPIURL, "http://localhost:8787")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIURL() != "http://localhost:8787" {
		t.Fatalf("环境变量应覆盖 API 地址，得到 %s", cfg.APIURL())
	}
}

func TestCorruptConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("FILEDROP_CONFIG", path)
	t.Setenv(EnvAPIURL, "")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil {
		t.Fatal("损坏的配置必须报错")
	}
}

func TestAPIURLNotPersisted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("FILEDROP_CONFIG", path)
	t.Setenv(EnvAPIURL, "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.SetAPIURL("http://localhost:8797/")
	if cfg.APIURL() != "http://localhost:8797" {
		t.Fatalf("应去掉末尾斜杠: %s", cfg.APIURL())
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if containsAPIURL(raw) {
		t.Fatalf("API 地址不应写进配置文件: %s", raw)
	}
}

func containsAPIURL(raw []byte) bool {
	return bytes.Contains(raw, []byte("localhost:8797")) || bytes.Contains(raw, []byte("apiUrl"))
}
