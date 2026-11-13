package api

import (
	"context"
	"net/http"
	"net/url"
)

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
