package filex

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/aimuz/filedrop-cli/internal/api"
	"github.com/aimuz/filedrop-cli/internal/crypto"
	"github.com/aimuz/filedrop-cli/internal/httpx"
	"github.com/aimuz/filedrop-cli/internal/term"
)

const (
	// maxAttempts 是单块的最大尝试次数（首次 + 3 次重试）。
	maxAttempts = 4
	// retryDelay 是退避起点：0.5s、1s、2s。
	retryDelay = 500 * time.Millisecond

	// defaultConcurrency 是同时下载的分块数。
	//
	// 单个 Range 请求的吞吐常常受限于单连接带宽，并发几个能让链路跑满；
	// 再多收益递减，而内存按 8 MiB/路 增长。
	defaultConcurrency = 4
)

// backoff 是重试前的等待，测试里换成空函数。
var backoff = sleep

// task 是一次下载：把一块块密文取回来、解密、写到该去的地方。
//
// 分块的所有上下文都收在这里，worker 只需要拿到块号。
type task struct {
	client        *api.Client
	fileID        string
	encryptedSize int64
	meta          crypto.Meta
	keys          *crypto.Keys
	out           *sink
	progress      *term.Progress

	// source 提供预签名地址，可被多个 worker 并发使用。
	source *downloadSource
}

// newTask 组装一次下载；downloadURL 是已经拿到的预签名地址。
func newTask(client *api.Client, fileID, downloadURL string, info *api.FileInfo, keys *crypto.Keys, meta crypto.Meta, out *sink, progress *term.Progress) *task {
	return &task{
		client:        client,
		fileID:        fileID,
		encryptedSize: info.EncryptedSize,
		meta:          meta,
		keys:          keys,
		out:           out,
		progress:      progress,
		source: newDownloadSource(downloadURL, func(ctx context.Context) (string, error) {
			url, err := client.Download(ctx, fileID)
			if err != nil {
				return "", downloadError(err)
			}
			return url, nil
		}),
	}
}

// chunk 是一块下载下来的密文，附带归还缓冲的方式。
type chunk struct {
	index  int
	sealed []byte
	err    error
	// recycle 把密文缓冲放回池里；解密或丢弃后必须调用一次。
	recycle func()
}

// Download 下载并解密，按块认证；任何一块试满次数仍失败就中止。
//
// 写盘方式取决于目标：
//   - 文件：每块解密后 WriteAt 到自己的偏移，块之间互不等待。
//   - 标准输出：管道只能顺序写，按块号排队输出。
func (t *task) Download(ctx context.Context, concurrency int) (err error) {
	if t.progress != nil {
		t.progress.Start("下载", t.meta.Size)
		defer func() {
			if err != nil {
				t.progress.Abort()
			} else {
				t.progress.Finish()
			}
		}()
	}

	workers := min(max(1, concurrency), t.meta.ChunkCount)

	// 快速失败：任何一块耗尽重试就取消其余 worker。
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	written, err := t.transfer(runCtx, workers, cancel)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if written != t.meta.Size {
		return errors.New("解密后的大小与元数据不一致，文件可能不完整")
	}
	return nil
}

func (t *task) transfer(ctx context.Context, workers int, cancel context.CancelFunc) (int64, error) {
	if t.out.stdout {
		return t.pipeline(ctx, workers, cancel)
	}
	return t.direct(ctx, workers, cancel)
}

// fetchAll 启动 N 个 worker 并行取块，返回结果通道；全部取完后通道关闭。
// 调用方必须消费到底，否则 worker 会阻塞在发送上。
func (t *task) fetchAll(ctx context.Context, workers int) <-chan chunk {
	buffers := make(chan []byte, workers)
	for range workers {
		buffers <- nil
	}

	jobs := make(chan int)
	results := make(chan chunk, workers)

	var workersWG sync.WaitGroup
	for range workers {
		workersWG.Go(func() {
			for index := range jobs {
				buffer := <-buffers
				sealed, err := t.source.fetch(ctx, buffer, index, t.encryptedSize, t.meta.ChunkCount)
				result := chunk{
					index:   index,
					sealed:  sealed,
					err:     err,
					recycle: func() { putBuffer(buffers, sealed[:0]) },
				}
				select {
				case results <- result:
				case <-ctx.Done():
					// 消费端已经走了：别把这一块留在通道里，也别再取新任务。
					putBuffer(buffers, sealed[:0])
					return
				}
				if err != nil {
					return
				}
			}
		})
	}

	go func() {
		defer close(jobs)
		for index := range t.meta.ChunkCount {
			select {
			case jobs <- index:
			case <-ctx.Done():
				return
			}
		}
	}()

	go func() {
		workersWG.Wait()
		close(results)
	}()
	return results
}

// direct 让每块各自落盘，块之间互不等待。
func (t *task) direct(ctx context.Context, workers int, cancel context.CancelFunc) (int64, error) {
	var (
		written  int64
		plain    []byte
		firstErr error
	)
	for item := range t.fetchAll(ctx, workers) {
		if firstErr != nil {
			// 已经失败了：其余的块直接归还缓冲，不再写盘。
			item.recycle()
			continue
		}
		if item.err != nil {
			// 网络错误要原样报出来；不能丢给解密器，否则会误报成「校验失败」。
			firstErr = t.chunkError(item)
			item.recycle()
			cancel()
			continue
		}
		opened, err := t.openChunk(&plain, item)
		if err == nil {
			err = t.out.writeAt(opened, int64(item.index)*crypto.ChunkSize)
		}
		if err != nil {
			firstErr = err
			cancel()
			continue
		}
		written += int64(len(opened))
		if t.progress != nil {
			t.progress.Add(int64(len(opened)))
		}
	}
	if firstErr != nil {
		return 0, firstErr
	}
	// 并行写留下的是「已写区域」，长度由最大偏移决定；
	// 统一截断到真实大小，防止最后一块更短时文件尾部带零。
	if written == t.meta.Size {
		if err := t.out.truncate(t.meta.Size); err != nil {
			return 0, err
		}
	}
	return written, nil
}

// pipeline 按块号顺序写进管道。标准输出无法随机写，只能排队。
func (t *task) pipeline(ctx context.Context, workers int, cancel context.CancelFunc) (int64, error) {
	pending := make(map[int]chunk, workers)
	next := 0
	var (
		written  int64
		plain    []byte
		firstErr error
	)
	for item := range t.fetchAll(ctx, workers) {
		if firstErr != nil {
			item.recycle()
			continue
		}
		if item.err != nil {
			firstErr = t.chunkError(item)
			item.recycle()
			cancel()
			continue
		}
		pending[item.index] = item
		for {
			ready, ok := pending[next]
			if !ok {
				break
			}
			delete(pending, next)
			opened, err := t.openChunk(&plain, ready)
			if err == nil {
				err = t.out.write(opened)
			}
			if err != nil {
				firstErr = err
				cancel()
				break
			}
			written += int64(len(opened))
			if t.progress != nil {
				t.progress.Add(int64(len(opened)))
			}
			next++
		}
	}
	for _, leftover := range pending {
		leftover.recycle()
	}
	if firstErr != nil {
		return 0, firstErr
	}
	return written, nil
}

// chunkError 把单块的下载错误补上块号；块级失败和解密失败要能区分开。
func (t *task) chunkError(item chunk) error {
	return fmt.Errorf("第 %d/%d 块下载失败: %w", item.index+1, t.meta.ChunkCount, item.err)
}

// openChunk 解密一块、裁掉最后一块的填充，并归还密文缓冲。
//
// plain 在调用之间复用：每块 8 MiB，不复用的话 1 GiB 文件要额外分配 1 GiB。
func (t *task) openChunk(plain *[]byte, item chunk) ([]byte, error) {
	last := item.index == t.meta.ChunkCount-1
	opened, err := t.keys.OpenChunkTo(*plain, t.fileID, item.index, last, item.sealed)
	// 密文缓冲立刻归还：接下来的写入用的是 opened，与 sealed 无关。
	item.recycle()
	if err != nil {
		return nil, fmt.Errorf("第 %d/%d 块校验失败: %w", item.index+1, t.meta.ChunkCount, err)
	}
	*plain = opened
	if last {
		if keep := t.meta.Size - int64(item.index)*crypto.ChunkSize; keep < int64(len(opened)) {
			return opened[:keep], nil
		}
	}
	return opened, nil
}

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
		return "", errors.New("文件已过期、已被删除，或已达到下载次数上限")
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

// putBuffer 把缓冲放回池里；池满时丢弃（worker 已经退出，不需要了）。
func putBuffer(pool chan []byte, buffer []byte) {
	select {
	case pool <- buffer:
	default:
	}
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

func (e *httpStatusError) Error() string { return fmt.Sprintf("下载失败: HTTP %d", e.code) }

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
		return nil, fmt.Errorf("下载失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		return nil, &httpStatusError{code: resp.StatusCode}
	}
	if resp.StatusCode == http.StatusOK && start != 0 {
		return nil, &fatalError{errors.New("对象存储忽略了 Range 请求，无法分块下载")}
	}

	want := end - start + 1
	if int64(cap(dst)) < want {
		dst = make([]byte, want)
	}
	dst = dst[:want]
	if _, err := io.ReadFull(resp.Body, dst); err != nil {
		return nil, fmt.Errorf("下载失败: %w", err)
	}
	return dst, nil
}
