package api

import (
	"context"
	"net/http"
	"net/url"
)

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
