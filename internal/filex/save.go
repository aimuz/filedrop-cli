// Package filex 实现「从链接或 ID 下载并解密」，包括口令模式。
package filex

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/aimuz/filedrop-cli/internal/api"
	"github.com/aimuz/filedrop-cli/internal/crypto"
	"github.com/aimuz/filedrop-cli/internal/term"
)

// Target 是从命令行参数解析出来的下载目标。
type Target struct {
	FileID string
	// Key 是链接 # 之后的密钥；为 nil 表示只有 ID（口令模式）。
	Key []byte
}

// ErrMissingKey 表示普通链接丢了 # 之后的密钥：服务端只有密文，谁也解不开。
var ErrMissingKey = errors.New(
	"这个链接缺少 # 之后的密钥，无法解密；请让发送方重新发一次完整链接",
)

var fileIDPattern = regexp.MustCompile(`(?:^|/)d/([A-Za-z0-9_-]{22})(?:[/?#]|$)`)
var bareIDPattern = regexp.MustCompile(`^(?:#)?([A-Za-z0-9_-]{22})(?:#(.*))?$`)

// ParseTarget 解析三种输入：
//   - 完整链接 https://filedrop.app/d/<id>#<key>
//   - <id>#<key>
//   - <id>（口令模式，需要输入口令）
func ParseTarget(raw string) (Target, error) {
	input := strings.TrimSpace(raw)
	if input == "" {
		return Target{}, errors.New("没有指定要下载的链接或 ID")
	}

	if strings.Contains(input, "://") {
		parsed, err := url.Parse(input)
		if err != nil {
			return Target{}, fmt.Errorf("链接格式不对: %w", err)
		}
		match := fileIDPattern.FindStringSubmatch(parsed.Path)
		if match == nil {
			return Target{}, errors.New("链接里找不到文件 ID（应为 /d/<id>）")
		}
		return targetWithKey(match[1], parsed.Fragment)
	}

	match := bareIDPattern.FindStringSubmatch(input)
	if match == nil {
		return Target{}, errors.New("无法识别这个链接或 ID")
	}
	return targetWithKey(match[1], match[2])
}

func targetWithKey(fileID, fragment string) (Target, error) {
	target := Target{FileID: fileID}
	if fragment == "" {
		return target, nil
	}
	key, err := crypto.FromBase64URL(fragment)
	if err != nil || len(key) != crypto.MasterKeyLength {
		return Target{}, errors.New("链接里的密钥格式不对，请确认链接完整")
	}
	target.Key = key
	return target, nil
}

// SaveOptions 控制下载。
type SaveOptions struct {
	// OutDir 是保存目录；"-" 表示写到标准输出。
	OutDir string
	// Password 在需要时询问口令（只有链接里没有密钥才会调用）。
	Password func() (string, error)
	// Progress 可以是 nil。
	Progress *term.Progress
}

// Result 是一次下载的结果。
type Result struct {
	Path string
	Meta crypto.Meta
}

// Save 下载、解密并写到磁盘。
func Save(ctx context.Context, client *api.Client, raw string, opts SaveOptions) (*Result, error) {
	target, err := ParseTarget(raw)
	if err != nil {
		return nil, err
	}

	info, err := client.FileInfo(ctx, target.FileID)
	if err != nil {
		var apiErr *api.Error
		if errors.As(err, &apiErr) && apiErr.Status == 404 {
			return nil, errors.New("文件不存在、已过期或已被删除")
		}
		return nil, err
	}

	masterKey := target.Key
	if masterKey == nil {
		if info.Wrap == nil {
			return nil, ErrMissingKey
		}
		if opts.Password == nil {
			return nil, errors.New("这个文件需要口令")
		}
		password, err := opts.Password()
		if err != nil {
			return nil, err
		}
		masterKey, err = crypto.UnwrapMasterKey(crypto.Wrap(*info.Wrap), password)
		if err != nil {
			return nil, err
		}
	}

	keys, err := crypto.DeriveKeys(masterKey, target.FileID)
	if err != nil {
		return nil, err
	}
	meta, err := keys.OpenMeta(target.FileID, info.MetaCipher)
	if err != nil {
		return nil, err
	}

	downloadURL, err := client.Download(ctx, target.FileID)
	if err != nil {
		return nil, err
	}

	out, err := openSink(opts.OutDir, meta.Name)
	if err != nil {
		return nil, err
	}
	succeeded := false
	defer func() {
		if !succeeded {
			out.discard()
		}
	}()

	if err := drain(ctx, target.FileID, downloadURL, info.EncryptedSize, meta, keys, out, opts.Progress); err != nil {
		return nil, err
	}
	if err := out.close(); err != nil {
		return nil, err
	}
	succeeded = true
	return &Result{Path: out.path, Meta: meta}, nil
}

// drain 按块 Range 下载并按块认证解密；任何一块失败都中止。
func drain(
	ctx context.Context,
	fileID, downloadURL string,
	encryptedSize int64,
	meta crypto.Meta,
	keys *crypto.Keys,
	out *sink,
	progress *term.Progress,
) error {
	if progress != nil {
		progress.Start("下载", meta.Size)
	}
	var written int64
	for index := 0; index < meta.ChunkCount; index++ {
		last := index == meta.ChunkCount-1
		start := int64(index) * crypto.CiphertextChunkSize
		end := start + crypto.CiphertextChunkSize - 1
		if last {
			end = encryptedSize - 1
		}
		sealed, err := fetchRange(ctx, downloadURL, start, end)
		if err != nil {
			return err
		}
		plain, err := keys.OpenChunk(fileID, index, last, sealed)
		if err != nil {
			return err
		}
		// 最后一块带随机填充，按元数据里的真实大小裁掉。
		content := plain
		if last {
			keep := meta.Size - int64(index)*crypto.ChunkSize
			if keep < int64(len(plain)) {
				content = plain[:keep]
			}
		}
		if err := out.write(content); err != nil {
			return err
		}
		written += int64(len(content))
		if progress != nil {
			progress.Update(written)
		}
	}
	if written != meta.Size {
		return errors.New("解密后的大小与元数据不一致，文件可能不完整")
	}
	if progress != nil {
		progress.Finish()
	}
	return nil
}

func fetchRange(ctx context.Context, rawURL string, start, end int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("range", fmt.Sprintf("bytes=%d-%d", start, end))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("下载失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("下载失败: HTTP %d", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusOK && start != 0 {
		return nil, errors.New("对象存储忽略了 Range 请求，无法分块下载")
	}
	return io.ReadAll(io.LimitReader(resp.Body, end-start+2))
}

// sink 是唯一的写出口：文件或标准输出。失败时丢弃已写内容。
type sink struct {
	file   *os.File
	stdout bool
	path   string
}

func (s *sink) write(p []byte) error {
	if s.stdout {
		_, err := os.Stdout.Write(p)
		return err
	}
	_, err := s.file.Write(p)
	return err
}

func (s *sink) close() error {
	if s.stdout {
		return nil
	}
	return s.file.Close()
}

func (s *sink) discard() {
	if s.stdout || s.file == nil {
		return
	}
	name := s.file.Name()
	s.file.Close()
	os.Remove(name)
}

// openSink 决定写到哪里。默认不覆盖同名文件：自动改成「name (1).ext」。
func openSink(outDir, name string) (*sink, error) {
	if outDir == "-" {
		return &sink{stdout: true, path: "-"}, nil
	}
	dir := outDir
	if dir == "" {
		dir = "."
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	base := filepath.Base(name)
	if base == "" || base == "." || base == ".." {
		base = "download"
	}
	path := uniquePath(dir, base)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	return &sink{file: f, path: path}, nil
}

// uniquePath 在不覆盖的前提下找一个可用文件名。
func uniquePath(dir, base string) string {
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	candidate := filepath.Join(dir, base)
	for i := 1; ; i++ {
		if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate
		}
		candidate = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", stem, i, ext))
	}
}
