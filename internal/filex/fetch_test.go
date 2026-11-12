package filex

import (
	"context"

	"github.com/aimuz/filedrop-cli/internal/api"
	"github.com/aimuz/filedrop-cli/internal/crypto"
)

// fetch 用给定地址把 sealed 下载到 out。
//
// 测试大多只关心「下载到指定目标」这一件事，这里把 task 的组装收起来；
// concurrency 为 0 表示用默认值。
func fetch(ctx context.Context, client *api.Client, fileID, downloadURL string, sealed []byte, meta crypto.Meta, keys *crypto.Keys, out *sink, concurrency int) error {
	info := &api.FileInfo{
		Status:        "active",
		EncryptedSize: int64(len(sealed)),
		ChunkCount:    meta.ChunkCount,
	}
	return newTask(client, fileID, downloadURL, info, keys, meta, out, nil).Download(ctx, concurrency)
}
