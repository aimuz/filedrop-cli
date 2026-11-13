package filex

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/aimuz/filedrop-cli/internal/crypto"
	"github.com/aimuz/filedrop-cli/internal/httpx"
	"github.com/aimuz/filedrop-cli/internal/i18n"
)

const (
	// maxAttempts 是单块的最大尝试次数（首次 + 3 次重试）。
	maxAttempts = 4
	// retryDelay 是退避起点：0.5s、1s、2s。
	retryDelay = 500 * time.Millisecond
)

// backoff 是重试前的等待，测试里换成空函数。
var backoff = sleep

// downloadSource 保存预签名地址，供多个 worker 并发使用。
type downloadSource struct {
	mu      sync.Mutex
	url     string
	renew   func(ctx context.Context) (string, error)
	renewed bool
}

func newDownloadSource(url string, renew func(context.Context) (string, error)) *downloadSource {
	return &downloadSource{url: url, renew: renew}
}

// current 返回可用的地址；为空时向服务端申请一次。
//
// 每个文件最多再申请一次：预签名地址一小时内有效，足够下完一个大文件；
// 每次申请都会消耗一次「下载次数」，所以不能每块都申请。
func (s *downloadSource) current(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.url != "" {
		return s.url, nil
	}
	if s.renewed {
		return "", errors.New(i18n.C().Filex.DownloadUnavailable)
	}
	s.renewed = true
	url, err := s.renew(ctx)
	if err != nil {
		return "", err
	}
	s.url = url
	return url, nil
}

// invalidate 在地址被对象存储拒绝时清空它，下次 current 会重新申请。
func (s *downloadSource) invalidate(url string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// 只清掉调用者用的那个，别把别人刚换上的新地址也清了。
	if s.url == url {
		s.url = ""
	}
}

// fetch 取第 index 块密文（重试 + 地址失效时换新），并复用 buffer。
func (s *downloadSource) fetch(ctx context.Context, buffer []byte, index int, encryptedSize int64, chunkCount int) ([]byte, error) {
	last := index == chunkCount-1
	start := int64(index) * crypto.CiphertextChunkSize
	end := start + crypto.CiphertextChunkSize - 1
	if last {
		end = encryptedSize - 1
	}

	var lastErr error
	for try := range maxAttempts {
		if try > 0 {
			if err := backoff(ctx, retryDelay<<(try-1)); err != nil {
				return nil, err
			}
		}
		url, err := s.current(ctx)
		if err != nil {
			return nil, err
		}
		body, err := fetchRange(ctx, buffer, url, start, end)
		if err == nil {
			return body, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if status, ok := errors.AsType[*httpStatusError](err); ok &&
			(status.code == http.StatusForbidden || status.code == http.StatusBadRequest) {
			s.invalidate(url)
			continue
		}
		if fatal, ok := errors.AsType[*fatalError](err); ok {
			return nil, fatal.err
		}
		lastErr = err
	}
	return nil, lastErr
}

func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// fatalError 标记不该重试的错误（例如对象存储说地址已过期）。
type fatalError struct{ err error }

func (e *fatalError) Error() string { return e.err.Error() }
func (e *fatalError) Unwrap() error { return e.err }

// httpStatusError 是对象存储返回的非 2xx 状态。
type httpStatusError struct{ code int }

func (e *httpStatusError) Error() string { return fmt.Sprintf(i18n.C().Filex.HTTPStatus, e.code) }

// fetchRange 读 [start, end] 这段密文到 dst（容量不足时扩容）。
func fetchRange(ctx context.Context, dst []byte, rawURL string, start, end int64) ([]byte, error) {
	// 必须带上 ctx：否则取消后正在传的块会继续占着连接，Ctrl-C 看起来「没反应」。
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, &fatalError{err}
	}
	req.Header.Set("range", fmt.Sprintf("bytes=%d-%d", start, end))
	resp, err := httpx.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf(i18n.C().Filex.TransferFailed, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		return nil, &httpStatusError{code: resp.StatusCode}
	}
	if resp.StatusCode == http.StatusOK && start != 0 {
		return nil, &fatalError{errors.New(i18n.C().Filex.RangeIgnored)}
	}

	want := end - start + 1
	if int64(cap(dst)) < want {
		dst = make([]byte, want)
	}
	dst = dst[:want]
	if _, err := io.ReadFull(resp.Body, dst); err != nil {
		return nil, fmt.Errorf(i18n.C().Filex.TransferFailed, err)
	}
	return dst, nil
}
