// Package api 是 filedrop 服务端 HTTP 接口的客户端。
//
// 与登录平台无关：这里只关心 Bearer 令牌。
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/aimuz/filedrop-cli/internal/i18n"
)

// Error 是服务端返回的结构化错误（{error:{code,message}}）。
type Error struct {
	Status  int
	Code    string
	Message string
}

func (e *Error) Error() string {
	if e.Code == "" {
		return fmt.Sprintf("HTTP %d", e.Status)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unauthorized 判断错误是否是未登录（令牌失效）。
func Unauthorized(err error) bool {
	apiErr, ok := errors.AsType[*Error](err)
	return ok && apiErr.Status == 401
}

// Client 调用服务端接口。BaseURL 末尾没有斜杠。
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
	// userAgent 里带系统与架构：授权页用它显示「macOS · Apple Silicon」，
	// 让用户能在确认前核对是不是自己这台机器。
	userAgent string
}

// defaultUserAgent 形如 `filedrop-cli/0.1.0 (darwin/arm64)`，服务端只解析括号里的部分。
func defaultUserAgent(version string) string {
	return fmt.Sprintf("filedrop-cli/%s (%s/%s)", version, runtime.GOOS, runtime.GOARCH)
}

// New 创建客户端。Token 可以为空（登录前的设备码接口）。
func New(baseURL, token string) *Client {
	return NewWithVersion(baseURL, token, "dev")
}

// NewWithVersion 与 New 相同，但把版本号写进 User-Agent。
func NewWithVersion(baseURL, token, version string) *Client {
	return &Client{
		BaseURL:   strings.TrimRight(baseURL, "/"),
		Token:     token,
		userAgent: defaultUserAgent(version),
		HTTP: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// WithToken 返回共享 HTTP 客户端与 User-Agent 的副本，只换令牌。
// 登录轮询拿到令牌后用它发请求，省得再拼一个版本号对不上的客户端。
func (c *Client) WithToken(token string) *Client {
	clone := *c
	clone.Token = token
	return &clone
}

type apiErrorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("content-type", "application/json")
	}
	req.Header.Set("accept", "application/json")
	req.Header.Set("user-agent", c.userAgent)
	if c.Token != "" {
		req.Header.Set("authorization", "Bearer "+c.Token)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf(i18n.C().API.ConnectFailed, c.BaseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeError(resp)
	}
	if out == nil {
		io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf(i18n.C().API.DecodeFailed, err)
	}
	return nil
}

func decodeError(resp *http.Response) error {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	var parsed apiErrorBody
	if json.Unmarshal(raw, &parsed) == nil && parsed.Error.Code != "" {
		return &Error{Status: resp.StatusCode, Code: parsed.Error.Code, Message: parsed.Error.Message}
	}
	return &Error{Status: resp.StatusCode, Message: strings.TrimSpace(string(raw))}
}
