// Package share 实现「加密后上传到 filedrop」，产出可以直接发给别人的链接。
package share

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/aimuz/filedrop-cli/internal/api"
	"github.com/aimuz/filedrop-cli/internal/crypto"
	"github.com/aimuz/filedrop-cli/internal/term"
)

// MaxFileSize 与网页版一致：单文件 1 GiB。
const MaxFileSize = 1024 * 1024 * 1024

// Options 控制一次分享。
type Options struct {
	// TTLSeconds 是保存时长；命令行固定 24 小时。
	TTLSeconds int
	// MaxDownloads 为 0 表示不限次数。
	MaxDownloads int
	// Password 非空时用口令包裹主密钥，链接里不带密钥。
	Password string
	// Progress 可以是 nil。
	Progress *term.Progress
}

// File 加密并上传一个文件，返回下载链接。
func File(ctx context.Context, client *api.Client, path string, opts Options) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", errors.New("不能分享目录，请指定文件")
	}
	if info.Size() > MaxFileSize {
		return "", fmt.Errorf("文件超过单文件上限 %s", term.Bytes(MaxFileSize))
	}

	masterKey, err := crypto.RandomBytes(crypto.MasterKeyLength)
	if err != nil {
		return "", err
	}

	// 先流式计算明文 SHA-256：服务端用它做重复与黑名单检查。
	digest, err := hashFile(path)
	if err != nil {
		return "", err
	}
	chunkCount := crypto.ExpectedChunkCount(info.Size())
	encryptedSize := crypto.EncryptedSizeFor(info.Size())

	var wrap *api.Wrap
	if opts.Password != "" {
		sealed, err := crypto.WrapMasterKey(masterKey, opts.Password)
		if err != nil {
			return "", err
		}
		wrap = &api.Wrap{KDF: sealed.KDF, Salt: sealed.Salt, WrappedKey: sealed.WrappedKey}
	}

	var maxDownloads *int
	if opts.MaxDownloads > 0 {
		value := opts.MaxDownloads
		maxDownloads = &value
	}

	created, err := client.CreateFile(ctx, api.CreateFileRequest{
		EncryptedSize: encryptedSize,
		ChunkCount:    chunkCount,
		SHA256:        digest,
		TTLSeconds:    opts.TTLSeconds,
		MaxDownloads:  maxDownloads,
		Wrap:          wrap,
	})
	if err != nil {
		return "", uploadError(err)
	}

	urls := make(map[int]string, len(created.PartURLs))
	for _, part := range created.PartURLs {
		urls[part.PartNumber] = part.URL
	}

	keys, err := crypto.DeriveKeys(masterKey, created.FileID)
	if err != nil {
		return "", err
	}
	meta := crypto.NewMeta(filepath.Base(path), info.Size(), contentType(path), chunkCount)
	metaCipher, err := keys.SealMeta(created.FileID, meta)
	if err != nil {
		return "", err
	}

	parts, err := uploadParts(ctx, client, created.FileID, path, info.Size(), keys, urls, opts.Progress)
	if err != nil {
		return "", err
	}
	if err := client.Complete(ctx, created.FileID, parts, metaCipher); err != nil {
		return "", uploadError(err)
	}

	base := strings.TrimRight(client.BaseURL, "/") + "/d/" + created.FileID
	if wrap != nil {
		return base, nil
	}
	return base + "#" + crypto.Base64URL(masterKey), nil
}

func uploadError(err error) error {
	var apiErr *api.Error
	if !errors.As(err, &apiErr) {
		return err
	}
	if apiErr.Code == "limit_exceeded" {
		return fmt.Errorf("%s（运行 filedrop quota 看容量，或用 filedrop rm 删除旧文件）", apiErr.Message)
	}
	return err
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

// contentType 写进加密元数据，供浏览器下载时提示。
func contentType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".pdf":
		return "application/pdf"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".txt", ".md", ".log":
		return "text/plain"
	case ".json":
		return "application/json"
	case ".zip":
		return "application/zip"
	case ".mp4":
		return "video/mp4"
	case ".mov":
		return "video/quicktime"
	case ".mp3":
		return "audio/mpeg"
	default:
		return "application/octet-stream"
	}
}

// uploadParts 边读边加密，逐块 PUT 到预签名地址；地址用完自动补签。
// 内存里同时只有一个明文块与一个密文块。
func uploadParts(
	ctx context.Context,
	client *api.Client,
	fileID string,
	path string,
	size int64,
	keys *crypto.Keys,
	urls map[int]string,
	progress *term.Progress,
) ([]api.CompletedPart, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	chunkCount := crypto.ExpectedChunkCount(size)
	lastPlainLength := crypto.PaddedLastChunkSize(size)
	plain := make([]byte, crypto.ChunkSize)
	completed := make([]api.CompletedPart, 0, chunkCount)

	if progress != nil {
		progress.Start("上传", size)
	}
	var sent int64

	for index := 0; index < chunkCount; index++ {
		last := index == chunkCount-1
		length := crypto.ChunkSize
		if last {
			length = lastPlainLength
		}
		contentLength := crypto.ChunkSize
		if last {
			contentLength = int(size - int64(index)*crypto.ChunkSize)
		}

		n, err := io.ReadFull(f, plain[:contentLength])
		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("读取 %s 失败: %w", path, err)
		}
		if n != contentLength {
			return nil, fmt.Errorf("读取 %s 失败：文件在传输期间被改动", path)
		}
		if last && length > contentLength {
			// 最后一块用随机字节补到 64 KiB 的整数倍（与网页版一致）。
			random, err := crypto.RandomBytes(length - contentLength)
			if err != nil {
				return nil, err
			}
			copy(plain[contentLength:length], random)
		}

		sealed, err := keys.SealChunk(fileID, index, last, plain[:length])
		if err != nil {
			return nil, err
		}

		partNumber := index + 1
		url := urls[partNumber]
		if url == "" {
			signed, err := client.Parts(ctx, fileID, []int{partNumber})
			if err != nil {
				return nil, err
			}
			if len(signed) == 0 {
				return nil, errors.New("服务端没有返回分片地址")
			}
			url = signed[0].URL
			urls[partNumber] = url
		}

		etag, err := putPart(ctx, url, sealed)
		if err != nil {
			return nil, err
		}
		completed = append(completed, api.CompletedPart{PartNumber: partNumber, ETag: etag})

		if progress != nil {
			sent += int64(contentLength)
			progress.Update(sent)
		}
	}
	if progress != nil {
		progress.Finish()
	}
	return completed, nil
}

func putPart(ctx context.Context, url string, body []byte) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.ContentLength = int64(len(body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("上传分片失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("上传分片失败: HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	etag := strings.Trim(resp.Header.Get("etag"), `"`)
	if etag == "" {
		return "", errors.New("对象存储没有返回 ETag（检查桶的 CORS 是否暴露 ETag）")
	}
	return etag, nil
}
