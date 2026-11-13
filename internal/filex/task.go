package filex

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/aimuz/filedrop-cli/internal/api"
	"github.com/aimuz/filedrop-cli/internal/crypto"
	"github.com/aimuz/filedrop-cli/internal/i18n"
	"github.com/aimuz/filedrop-cli/internal/term"
)

// defaultConcurrency 是同时下载的分块数。
//
// 单个 Range 请求的吞吐常常受限于单连接带宽，并发几个能让链路跑满；
// 再多收益递减，而内存按 8 MiB/路 增长。
const defaultConcurrency = 4

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
		t.progress.Start(i18n.C().Filex.ProgressLabel, t.meta.Size)
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
		return errors.New(i18n.C().Filex.SizeMismatch)
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
	return fmt.Errorf(i18n.C().Filex.ChunkDownloadFailed, item.index+1, t.meta.ChunkCount, item.err)
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
		return nil, fmt.Errorf(i18n.C().Filex.ChunkVerifyFailed, item.index+1, t.meta.ChunkCount, err)
	}
	*plain = opened
	if last {
		if keep := t.meta.Size - int64(item.index)*crypto.ChunkSize; keep < int64(len(opened)) {
			return opened[:keep], nil
		}
	}
	return opened, nil
}

// putBuffer 把缓冲放回池里；池满时丢弃（worker 已经退出，不需要了）。
func putBuffer(pool chan []byte, buffer []byte) {
	select {
	case pool <- buffer:
	default:
	}
}
