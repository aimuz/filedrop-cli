// Package crypto 实现 filedrop 的端到端加密协议，与 @filedrop/crypto 逐字节兼容。
//
// 协议要点（与 packages/crypto 保持一致）：
//   - 主密钥 K：32 字节随机数，只存在于下载链接的 # 之后或内存里；
//   - 文件密钥：HKDF-SHA256(K, salt=fileId, info="filedrop/v1/file")；
//   - 元数据密钥：HKDF-SHA256(K, salt=fileId, info="filedrop/v1/meta")；
//   - 分块：8 MiB 一块，AES-256-GCM，nonce = 4 字节零 + 8 字节大端块序号，
//     AAD = "filedrop/v1" ‖ fileId(16) ‖ u32 块序号 ‖ u8 是否最后一块；
//   - 最后一块明文补到 64 KiB 的整数倍（随机填充），服务端看不到精确大小；
//   - 元数据固定补到 4 KiB 后加密，AAD = "filedrop/v1" ‖ fileId；
//   - 口令模式：PBKDF2-HMAC-SHA256(600k) 派生 KEK，AAD = "filedrop/v1/key"。
package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/aimuz/filedrop-cli/internal/i18n"
)

const (
	// ChunkSize 是每块的明文大小：8 MiB。
	ChunkSize = 8 * 1024 * 1024
	// TagLength 是 AES-GCM 认证标签长度。
	TagLength = 16
	// CiphertextChunkSize 是除最后一块外的密文长度。
	CiphertextChunkSize = ChunkSize + TagLength
	// PadBlock 是最后一块的填充粒度：64 KiB。
	PadBlock = 64 * 1024
	// MasterKeyLength 是主密钥长度。
	MasterKeyLength = 32
	// FileIDLength 是 fileId 解码后的字节数。
	FileIDLength = 16

	protocolVersion = "filedrop/v1"
	nonceLength     = 12
	metaPlainSize   = 4096

	// PBKDF2Iterations 是口令模式默认迭代次数（与 protocol 的 pbkdf2-sha256-600k 一致）。
	PBKDF2Iterations = 600_000
)

// FormatError 表示数据格式不对（链接、元数据、口令），不是网络错误。
type FormatError struct{ Reason string }

func (e *FormatError) Error() string { return e.Reason }

// formatError 用于不带占位符的文案；带参数的走 formatErrorf。
func formatError(message string) error {
	return &FormatError{Reason: message}
}

func formatErrorf(format string, args ...any) error {
	return &FormatError{Reason: fmt.Sprintf(format, args...)}
}

// ErrWrongPassword 是口令解不开主密钥时的错误。
// 文案延迟取当前语言，但错误值本身保持不变，便于比较与 errors.Is。
var ErrWrongPassword = i18n.NewError(func() string { return i18n.C().Crypto.WrongPassword })

// Base64URL 编码，不带 = 填充。
func Base64URL(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// FromBase64URL 解码。链接常被手工复制，可能带 = 填充或标准字母表（+/），
// 这里都容忍；真正的长度校验由调用者完成。
func FromBase64URL(s string) ([]byte, error) {
	if s == "" {
		return nil, nil
	}
	s = strings.TrimRight(s, "=")
	if strings.ContainsAny(s, "+/") {
		s = strings.NewReplacer("+", "-", "/", "_").Replace(s)
	}
	return base64.RawURLEncoding.DecodeString(s)
}

// RandomBytes 返回 n 个加密安全的随机字节。
func RandomBytes(n int) ([]byte, error) {
	out := make([]byte, n)
	if _, err := rand.Read(out); err != nil {
		return nil, err
	}
	return out, nil
}

// FileIDBytes 校验并解码 22 字符的 fileId。
func FileIDBytes(fileID string) ([]byte, error) {
	if len(fileID) != 22 {
		return nil, formatError(i18n.C().Crypto.BadFileID)
	}
	raw, err := FromBase64URL(fileID)
	if err != nil || len(raw) != FileIDLength {
		return nil, formatError(i18n.C().Crypto.BadFileID)
	}
	return raw, nil
}

// ExpectedChunkCount 是明文按 8 MiB 切块的块数（空文件也算一块）。
func ExpectedChunkCount(size int64) int {
	if size <= 0 {
		return 1
	}
	return int((size + ChunkSize - 1) / ChunkSize)
}

// PaddedLastChunkSize 是最后一块补齐后的明文长度。
func PaddedLastChunkSize(size int64) int {
	if size <= 0 {
		return PadBlock
	}
	remainder := size % ChunkSize
	if remainder == 0 {
		return ChunkSize
	}
	padded := (remainder + PadBlock - 1) / PadBlock * PadBlock
	if padded > ChunkSize {
		return ChunkSize
	}
	return int(padded)
}

// EncryptedSizeFor 是给定明文大小后，对象存储里应有的密文总长度。
func EncryptedSizeFor(size int64) int64 {
	chunks := int64(ExpectedChunkCount(size))
	return (chunks-1)*CiphertextChunkSize + int64(PaddedLastChunkSize(size)) + TagLength
}
