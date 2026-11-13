// Package filex 实现「从链接或 ID 下载并解密」，包括口令模式。
package filex

import (
	"context"
	"errors"

	"github.com/aimuz/filedrop-cli/internal/api"
	"github.com/aimuz/filedrop-cli/internal/crypto"
	"github.com/aimuz/filedrop-cli/internal/term"
)

// SaveOptions 控制下载。
type SaveOptions struct {
	// OutDir 是保存目录；"-" 表示写到标准输出。
	OutDir string
	// Password 在需要时询问口令（只有链接里没有密钥才会调用）。
	Password func() (string, error)
	// Progress 可以是 nil。
	Progress *term.Progress
	// Concurrency 是同时下载的分块数；0 表示用默认值。
	Concurrency int
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

	info, masterKey, err := openFile(ctx, client, target, opts.Password)
	if err != nil {
		return nil, err
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
		return nil, downloadError(err)
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

	if err := newTask(client, target.FileID, downloadURL, info, keys, meta, out, opts.Progress).Download(ctx, opts.Concurrency); err != nil {
		return nil, err
	}
	if err := out.close(); err != nil {
		return nil, err
	}
	succeeded = true
	return &Result{Path: out.path, Meta: meta}, nil
}

// openFile 查询文件信息，并拿到主密钥：链接里带了就用链接里的，
// 否则用服务端的 wrap 加口令解出来。
func openFile(ctx context.Context, client *api.Client, target Target, password func() (string, error)) (*api.FileInfo, []byte, error) {
	info, err := client.FileInfo(ctx, target.FileID)
	if err != nil {
		if apiErr, ok := errors.AsType[*api.Error](err); ok && apiErr.Status == 404 {
			return nil, nil, errors.New("文件不存在、已过期或已被删除")
		}
		return nil, nil, err
	}
	if target.Key != nil {
		return info, target.Key, nil
	}
	if info.Wrap == nil {
		return nil, nil, ErrMissingKey
	}
	if password == nil {
		return nil, nil, errors.New("这个文件需要口令")
	}
	pass, err := password()
	if err != nil {
		return nil, nil, err
	}
	masterKey, err := crypto.UnwrapMasterKey(crypto.Wrap(*info.Wrap), pass)
	if err != nil {
		return nil, nil, err
	}
	return info, masterKey, nil
}

// downloadError 把「下载次数用完」「文件已不存在」讲清楚。
func downloadError(err error) error {
	if apiErr, ok := errors.AsType[*api.Error](err); ok && apiErr.Status == 404 {
		return errors.New("文件已过期、已被删除，或已达到下载次数上限")
	}
	return err
}
