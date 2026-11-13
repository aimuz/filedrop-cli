package share

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/aimuz/filedrop-cli/internal/api"
	"github.com/aimuz/filedrop-cli/internal/crypto"
	"github.com/aimuz/filedrop-cli/internal/httpx"
	"github.com/aimuz/filedrop-cli/internal/i18n"
	"github.com/aimuz/filedrop-cli/internal/term"
)

// uploadConcurrency 是同时上传的分片数。
//
// 单个 PUT 的吞吐受单连接带宽与 RTT 限制，并行几个能让链路跑满；
// 内存按 8 MiB/路 增长，4 路对普通机器已经是合适的上限。
const uploadConcurrency = 4

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

	return link(content.Base, created.FileID, masterKey, wrap != nil), nil
}

// uploadParts 边读边加密，按窗口并发 PUT；地址用完自动补签。
//
// 明文严格按块序读取，密文缓冲按窗口复用：内存里最多只有
// uploadConcurrency 个密文块，外加一个明文块。
func uploadParts(
	ctx context.Context,
	client *api.Client,
	fileID string,
	content *Content,
	keys *crypto.Keys,
	urls map[int]string,
	progress *term.Progress,
) (parts []api.CompletedPart, err error) {
	if progress != nil {
		progress.Start(i18n.C().Share.ProgressLabel, content.size)
		defer func() {
			if err != nil {
				progress.Abort()
			} else {
				progress.Finish()
			}
		}()
	}

	r, err := content.open()
	if err != nil {
		return nil, err
	}
	defer r.Close()

	size := content.size
	chunkCount := crypto.ExpectedChunkCount(size)
	workers := min(uploadConcurrency, chunkCount)

	plain := make([]byte, crypto.ChunkSize)
	// sealedBufs 按 worker 复用密文缓冲：每块 8 MiB，不复用会让 1 GiB 文件
	// 在上传期间反复分配、给 GC 制造很大压力。
	sealedBufs := make([][]byte, workers)
	for i := range sealedBufs {
		sealedBufs[i] = make([]byte, 0, crypto.CiphertextChunkSize)
	}
	completed := make([]api.CompletedPart, 0, chunkCount)
	var sent int64

	for start := 0; start < chunkCount; start += workers {
		end := min(start+workers, chunkCount)
		if err := signParts(ctx, client, fileID, urls, start, end); err != nil {
			return nil, err
		}

		batch := make([][]byte, end-start)
		plainLengths := make([]int64, end-start)
		for index := start; index < end; index++ {
			last := index == chunkCount-1
			length := crypto.ChunkSize
			if last {
				length = crypto.PaddedLastChunkSize(size)
			}
			contentLength := crypto.ChunkSize
			if last {
				contentLength = int(size - int64(index)*crypto.ChunkSize)
			}

			n, err := io.ReadFull(r, plain[:contentLength])
			if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
				return nil, fmt.Errorf(i18n.C().Share.ReadFailed, content.Name, err)
			}
			if n != contentLength {
				return nil, fmt.Errorf(i18n.C().Share.FileChanged, content.Name)
			}
			if last && length > contentLength {
				// 最后一块用随机字节补到 64 KiB 的整数倍（与网页版一致）。
				random, err := crypto.RandomBytes(length - contentLength)
				if err != nil {
					return nil, err
				}
				copy(plain[contentLength:length], random)
			}

			slot := index - start
			sealed, err := keys.SealChunkTo(sealedBufs[slot], fileID, index, last, plain[:length])
			if err != nil {
				return nil, err
			}
			batch[slot] = sealed
			plainLengths[slot] = int64(contentLength)
		}

		etags := make([]string, len(batch))
		errs := make([]error, len(batch))
		var wg sync.WaitGroup
		for offset := range batch {
			partNumber := start + offset + 1
			url := urls[partNumber]
			wg.Go(func() {
				etags[offset], errs[offset] = putPart(ctx, url, batch[offset])
			})
		}
		wg.Wait()

		for offset, err := range errs {
			if err != nil {
				return nil, err
			}
			completed = append(completed, api.CompletedPart{
				PartNumber: start + offset + 1,
				ETag:       etags[offset],
			})
			if progress != nil {
				sent += plainLengths[offset]
				progress.Update(sent)
			}
		}
	}
	return completed, nil
}

// signParts 把 [start, end) 区间里还缺地址的分片一次性补签。
// 批量补签比逐块请求少很多往返，也避免并发 worker 各签各的。
func signParts(ctx context.Context, client *api.Client, fileID string, urls map[int]string, start, end int) error {
	var missing []int
	for index := start; index < end; index++ {
		if partNumber := index + 1; urls[partNumber] == "" {
			missing = append(missing, partNumber)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	if client == nil {
		return errors.New(i18n.C().Share.MissingPartURL)
	}
	signed, err := client.Parts(ctx, fileID, missing)
	if err != nil {
		return err
	}
	for _, part := range signed {
		urls[part.PartNumber] = part.URL
	}
	for _, partNumber := range missing {
		if urls[partNumber] == "" {
			return errors.New(i18n.C().Share.MissingPartURL)
		}
	}
	return nil
}

func putPart(ctx context.Context, url string, body []byte) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.ContentLength = int64(len(body))
	resp, err := httpx.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf(i18n.C().Share.UploadPartFailed, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf(i18n.C().Share.UploadPartStatus, resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	etag := strings.Trim(resp.Header.Get("etag"), `"`)
	if etag == "" {
		return "", errors.New(i18n.C().Share.MissingETag)
	}
	return etag, nil
}
