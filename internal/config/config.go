package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aimuz/filedrop-cli/internal/i18n"
)

// DefaultAPIURL 是默认 API 地址，可用环境变量 FILEDROP_API_URL 覆盖。
const DefaultAPIURL = "https://filedrop.app"

// EnvAPIURL 覆盖 API 地址的环境变量名。
const EnvAPIURL = "FILEDROP_API_URL"

// Config 是持久化到磁盘的内容：只有令牌和账号展示名。
// API 地址刻意不落盘——它只来自默认值或 FILEDROP_API_URL，
// 这样临时指向测试服务器后，后续命令不会莫名沿用旧地址。
type Config struct {
	Token string `json:"token,omitempty"`
	// Account 只用于 whoami 的提示，随时可以由服务端重新获取。
	Account string `json:"account,omitempty"`

	path   string
	apiURL string
}

// Path 返回配置文件路径：
//   - $FILEDROP_CONFIG 显式指定时用它；
//   - macOS 用 ~/Library/Application Support/filedrop/config.json；
//   - 其他系统用 $XDG_CONFIG_HOME/filedrop/config.json（缺省 ~/.config）。
func Path() (string, error) {
	if custom := os.Getenv("FILEDROP_CONFIG"); custom != "" {
		return custom, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf(i18n.C().Config.NoConfigDir, err)
	}
	return filepath.Join(dir, "filedrop", "config.json"), nil
}

// Load 读取配置；文件不存在时返回空配置，不报错。
func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	cfg := &Config{path: path, apiURL: DefaultAPIURL}
	raw, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(raw, cfg); err != nil {
			return nil, fmt.Errorf(i18n.C().Config.Corrupt, path, err)
		}
	case errors.Is(err, os.ErrNotExist):
		// 首次运行。
	default:
		return nil, err
	}
	cfg.path = path
	if env := os.Getenv(EnvAPIURL); env != "" {
		cfg.apiURL = trimSlash(env)
	}
	return cfg, nil
}

// APIURL 返回当前生效的 API 地址。
func (c *Config) APIURL() string { return c.apiURL }

func trimSlash(url string) string {
	for len(url) > 1 && url[len(url)-1] == '/' {
		url = url[:len(url)-1]
	}
	return url
}

// Save 原子写入配置文件：先写临时文件再 rename，权限 0600。
func (c *Config) Save() error {
	if c.path == "" {
		path, err := Path()
		if err != nil {
			return err
		}
		c.path = path
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(c.path), ".config-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), c.path)
}
