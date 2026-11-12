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
	"net/url"
	"runtime"
	"strings"
	"time"
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
	var apiErr *Error
	return errors.As(err, &apiErr) && apiErr.Status == 401
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
		return fmt.Errorf("连接 %s 失败: %w", c.BaseURL, err)
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
		return fmt.Errorf("解析响应失败: %w", err)
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

// SessionInfo 是 /api/auth/session 的响应。
type SessionInfo struct {
	UserID      string `json:"userId"`
	DisplayName string `json:"displayName"`
}

// Session 返回当前登录账号。
func (c *Client) Session(ctx context.Context) (*SessionInfo, error) {
	var out SessionInfo
	if err := c.do(ctx, http.MethodGet, "/api/auth/session", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Logout 撤销当前令牌。
func (c *Client) Logout(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/api/auth/logout", nil, nil)
}

// AccountInfo 是 /api/account 的响应。
type AccountInfo struct {
	UsedBytes  int64 `json:"usedBytes"`
	QuotaBytes int64 `json:"quotaBytes"`
	FileCount  int   `json:"fileCount"`
}

// Account 返回账号容量。
func (c *Client) Account(ctx context.Context) (*AccountInfo, error) {
	var out AccountInfo
	if err := c.do(ctx, http.MethodGet, "/api/account", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AccountFile 是账号下的一个文件。按设计只有这三个字段。
type AccountFile struct {
	FileID        string `json:"fileId"`
	EncryptedSize int64  `json:"encryptedSize"`
	ExpiresAt     int64  `json:"expiresAt"`
}

// ListFiles 列出账号下未过期的文件。
func (c *Client) ListFiles(ctx context.Context) ([]AccountFile, error) {
	var out struct {
		Files []AccountFile `json:"files"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/account/files", nil, &out); err != nil {
		return nil, err
	}
	return out.Files, nil
}

// DeleteFile 删除账号下的文件。
func (c *Client) DeleteFile(ctx context.Context, fileID string) error {
	return c.do(ctx, http.MethodDelete, "/api/account/files/"+url.PathEscape(fileID), nil, nil)
}

// Wrap 是口令模式的外壳：服务端只保存这些字段，无法还原主密钥。
type Wrap struct {
	KDF        string `json:"kdf"`
	Salt       string `json:"salt"`
	WrappedKey string `json:"wrappedKey"`
}

// FileInfo 是 /api/files/:id 的响应。
type FileInfo struct {
	Status        string `json:"status"`
	EncryptedSize int64  `json:"encryptedSize"`
	ChunkCount    int    `json:"chunkCount"`
	MetaCipher    string `json:"metaCiphertext"`
	Wrap          *Wrap  `json:"wrap"`
	ExpiresAt     int64  `json:"expiresAt"`
}

// FileInfo 查询文件信息。不存在/过期返回 *Error（404）。
func (c *Client) FileInfo(ctx context.Context, fileID string) (*FileInfo, error) {
	var out FileInfo
	if err := c.do(ctx, http.MethodGet, "/api/files/"+url.PathEscape(fileID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PresignedPart 是一个分片的上传地址。
type PresignedPart struct {
	PartNumber int    `json:"partNumber"`
	URL        string `json:"url"`
}

// CreateFileRequest 是 POST /api/files 的请求体。
type CreateFileRequest struct {
	EncryptedSize int64  `json:"encryptedSize"`
	ChunkCount    int    `json:"chunkCount"`
	SHA256        string `json:"sha256"`
	TTLSeconds    int    `json:"ttlSeconds"`
	MaxDownloads  *int   `json:"maxDownloads"`
	Wrap          *Wrap  `json:"wrap"`
}

// CreateFileResponse 是 POST /api/files 的响应。
type CreateFileResponse struct {
	FileID      string          `json:"fileId"`
	DeleteToken string          `json:"deleteToken"`
	UploadID    string          `json:"uploadId"`
	PartURLs    []PresignedPart `json:"partUrls"`
	ChunkCount  int             `json:"chunkCount"`
	ExpiresAt   int64           `json:"expiresAt"`
}

// CreateFile 创建分片上传并拿到前几个分片地址。
func (c *Client) CreateFile(ctx context.Context, req CreateFileRequest) (*CreateFileResponse, error) {
	var out CreateFileResponse
	if err := c.do(ctx, http.MethodPost, "/api/files", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Parts 为后续分片补签上传地址。
func (c *Client) Parts(ctx context.Context, fileID string, numbers []int) ([]PresignedPart, error) {
	var out struct {
		PartURLs []PresignedPart `json:"partUrls"`
	}
	body := map[string]any{"partNumbers": numbers}
	if err := c.do(ctx, http.MethodPost, "/api/files/"+url.PathEscape(fileID)+"/parts", body, &out); err != nil {
		return nil, err
	}
	return out.PartURLs, nil
}

// CompletedPart 是完成上传时提交的分片。
type CompletedPart struct {
	PartNumber int    `json:"partNumber"`
	ETag       string `json:"etag"`
}

// Complete 完成分片上传并提交加密元数据。
func (c *Client) Complete(ctx context.Context, fileID string, parts []CompletedPart, metaCipher string) error {
	body := map[string]any{"parts": parts, "metaCiphertext": metaCipher}
	return c.do(ctx, http.MethodPost, "/api/files/"+url.PathEscape(fileID)+"/complete", body, nil)
}

// Download 申请下载地址。
func (c *Client) Download(ctx context.Context, fileID string) (string, error) {
	var out struct {
		URL string `json:"url"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/files/"+url.PathEscape(fileID)+"/download", nil, &out); err != nil {
		return "", err
	}
	return out.URL, nil
}

// DeviceStart 是 POST /api/auth/device 的响应。
type DeviceStart struct {
	Code      string `json:"code"`
	AuthURL   string `json:"authUrl"`
	Interval  int    `json:"interval"`
	ExpiresAt int64  `json:"expiresAt"`
}

// DeviceStart 申请一个登录票据。
func (c *Client) DeviceStart(ctx context.Context, clientName string) (*DeviceStart, error) {
	var out DeviceStart
	if err := c.do(ctx, http.MethodPost, "/api/auth/device", map[string]string{"clientName": clientName}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DevicePoll 是 GET /api/auth/device 的响应。
type DevicePoll struct {
	Status string `json:"status"`
	Token  string `json:"token"`
}

// DevicePoll 轮询登录票据状态。
func (c *Client) DevicePoll(ctx context.Context, code string) (*DevicePoll, error) {
	var out DevicePoll
	path := "/api/auth/device?code=" + url.QueryEscape(code)
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
