package share

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"sync"
)

// hashBufferSize 是整文件哈希的读缓冲。
//
// io.Copy 默认只用 32 KB，SHA-256 的 SIMD 实现按块搬运数据，
// 小写入会把吞吐压到 ~2 GB/s；1 MB 缓冲能到 ~3 GB/s（接近纯内存哈希的上限）。
const hashBufferSize = 1 << 20

// hashBuffers 复用哈希读缓冲，避免每次分享都分配 1 MB。
var hashBuffers = sync.Pool{
	New: func() any {
		buffer := make([]byte, hashBufferSize)
		return &buffer
	},
}

// hashContent 流式计算明文 SHA-256（小写 hex），服务端用它做重复与黑名单检查。
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
