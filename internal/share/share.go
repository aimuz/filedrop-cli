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
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/aimuz/filedrop-cli/internal/api"
	"github.com/aimuz/filedrop-cli/internal/config"
	"github.com/aimuz/filedrop-cli/internal/crypto"
	"github.com/aimuz/filedrop-cli/internal/term"
)

// MaxFileSize 与网页版一致：单文件 1 GiB。
const MaxFileSize = 1024 * 1024 * 1024

// Content 是要分享的内容：磁盘上的文件，或者来自剪贴板的文本。
//
// Name 是放进加密元数据的文件名（会写进对方的下载目录），Base 是输出链接所用的地址。
type Content struct {
	Name string
	Base string
	open func() (io.ReadCloser, error)
	size int64
}

// FromPath 指向磁盘上的文件。
func FromPath(path, base string) (*Content, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, errors.New("不能分享目录，请指定文件")
	}
	if info.Size() > MaxFileSize {
		return nil, fmt.Errorf("文件超过单文件上限 %s", term.Bytes(MaxFileSize))
	}
	return &Content{
		Name: filepath.Base(path),
		Base: base,
		size: info.Size(),
		open: func() (io.ReadCloser, error) { return os.Open(path) },
	}, nil
}

// textName 是剪贴板文本的默认文件名；扩展名由内容猜出来，方便对方双击打开。
func textName(text string) string {
	trimmed := strings.TrimSpace(text)
	switch {
	case strings.HasPrefix(trimmed, "{"), strings.HasPrefix(trimmed, "["):
		return "clipboard.json"
	case strings.HasPrefix(trimmed, "<"):
		return "clipboard.html"
	default:
		return "clipboard.txt"
	}
}

// FromText 把一段文本当作文件分享（例如 `filedrop share --clipboard`）。
func FromText(text, base string) (*Content, error) {
	size := int64(len(text))
	if size > MaxFileSize {
		return nil, fmt.Errorf("内容超过单文件上限 %s", term.Bytes(MaxFileSize))
	}
	return &Content{
		Name: textName(text),
		Base: base,
		size: size,
		open: func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(text)), nil },
	}, nil
}

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
	content, err := FromPath(path, client.BaseURL)
	if err != nil {
		return "", err
	}
	return Upload(ctx, client, content, opts)
}

// Link 拼出下载链接；wrap 为 nil 时把主密钥放在 # 之后。
func Link(base, fileID string, masterKey []byte, wrapped bool) string {
	base = strings.TrimRight(base, "/")
	if base == "" {
		base = config.DefaultAPIURL
	}
	link := base + "/d/" + fileID
	if wrapped {
		return link
	}
	return link + "#" + crypto.Base64URL(masterKey)
}

// Upload 加密并上传，返回下载链接。
func Upload(ctx context.Context, client *api.Client, content *Content, opts Options) (string, error) {
	masterKey, err := crypto.RandomBytes(crypto.MasterKeyLength)
	if err != nil {
		return "", err
	}

	// 先流式计算明文 SHA-256：服务端用它做重复与黑名单检查。
	digest, err := hashContent(content)
	if err != nil {
		return "", err
	}
	chunkCount := crypto.ExpectedChunkCount(content.size)
	encryptedSize := crypto.EncryptedSizeFor(content.size)

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
		maxDownloads = new(opts.MaxDownloads)
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
	meta := crypto.NewMeta(content.Name, content.size, contentType(content.Name), chunkCount)
	metaCipher, err := keys.SealMeta(created.FileID, meta)
	if err != nil {
		return "", err
	}

	parts, err := uploadParts(ctx, client, created.FileID, content, keys, urls, opts.Progress)
	if err != nil {
		return "", err
	}
	if err := client.Complete(ctx, created.FileID, parts, metaCipher); err != nil {
		return "", uploadError(err)
	}

	return Link(content.Base, created.FileID, masterKey, wrap != nil), nil
}

func uploadError(err error) error {
	apiErr, ok := errors.AsType[*api.Error](err)
	if !ok {
		return err
	}
	if apiErr.Code == "limit_exceeded" {
		// 服务端会给出「已用 x of y bytes」；本地化成人能读的提示。
		if used, quota, ok := parseUsage(apiErr.Message); ok {
			return fmt.Errorf(
				"账号容量不足：已用 %s / %s。先运行 filedrop rm <ID> 删除旧文件，或等它们过期",
				term.Bytes(used), term.Bytes(quota),
			)
		}
		return fmt.Errorf("%s（运行 filedrop quota 看容量，或用 filedrop rm 删除旧文件）", apiErr.Message)
	}
	return err
}

var usagePattern = regexp.MustCompile(`account storage full: (\d+) of (\d+) bytes used`)

// parseUsage 解析服务端配额错误里的「已用/上限」字节数。
func parseUsage(message string) (used, quota int64, ok bool) {
	match := usagePattern.FindStringSubmatch(message)
	if match == nil {
		return 0, 0, false
	}
	used, err1 := strconv.ParseInt(match[1], 10, 64)
	quota, err2 := strconv.ParseInt(match[2], 10, 64)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return used, quota, true
}

const (
	// hashBufferSize 是整文件哈希的读缓冲。
	//
	// io.Copy 默认只用 32 KB，SHA-256 的 SIMD 实现按块搬运数据，
	// 小写入会把吞吐压到 ~2 GB/s；1 MB 缓冲能到 ~3 GB/s（接近纯内存哈希的上限）。
	hashBufferSize = 1 << 20
)

// hashBuffers 复用哈希读缓冲，避免每次分享都分配 1 MB。
var hashBuffers = sync.Pool{
	New: func() any {
		buffer := make([]byte, hashBufferSize)
		return &buffer
	},
}

func hashContent(content *Content) (string, error) {
	r, err := content.open()
	if err != nil {
		return "", err
	}
	defer r.Close()

	buffer := hashBuffers.Get().(*[]byte)
	defer hashBuffers.Put(buffer)

	hasher := sha256.New()
	if _, err := io.CopyBuffer(hasher, r, *buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
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
	case ".html", ".htm":
		return "text/html"
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
	content *Content,
	keys *crypto.Keys,
	urls map[int]string,
	progress *term.Progress,
) ([]api.CompletedPart, error) {
	r, err := content.open()
	if err != nil {
		return nil, err
	}
	defer r.Close()

	size := content.size
	chunkCount := crypto.ExpectedChunkCount(size)
	lastPlainLength := crypto.PaddedLastChunkSize(size)
	plain := make([]byte, crypto.ChunkSize)
	// 密文缓冲区复用：每块 8 MiB，不复用的话 1 GiB 文件要额外分配 1 GiB 内存。
	sealed := make([]byte, 0, crypto.CiphertextChunkSize)
	completed := make([]api.CompletedPart, 0, chunkCount)

	if progress != nil {
		progress.Start("上传", size)
	}
	var sent int64

	for index := range chunkCount {
		last := index == chunkCount-1
		length := crypto.ChunkSize
		if last {
			length = lastPlainLength
		}
		contentLength := crypto.ChunkSize
		if last {
			contentLength = int(size - int64(index)*crypto.ChunkSize)
		}

		n, err := io.ReadFull(r, plain[:contentLength])
		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("读取 %s 失败: %w", content.Name, err)
		}
		if n != contentLength {
			return nil, fmt.Errorf("读取 %s 失败：文件在传输期间被改动", content.Name)
		}
		if last && length > contentLength {
			// 最后一块用随机字节补到 64 KiB 的整数倍（与网页版一致）。
			random, err := crypto.RandomBytes(length - contentLength)
			if err != nil {
				return nil, err
			}
			copy(plain[contentLength:length], random)
		}

		sealed, err = keys.SealChunkTo(sealed, fileID, index, last, plain[:length])
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
