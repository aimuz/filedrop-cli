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
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
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

func formatError(format string, args ...any) error {
	return &FormatError{Reason: fmt.Sprintf(format, args...)}
}

// ErrWrongPassword 是口令解不开主密钥时的错误。
var ErrWrongPassword = errors.New("口令不正确")

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
		return nil, formatError("文件 ID 格式不对")
	}
	raw, err := FromBase64URL(fileID)
	if err != nil || len(raw) != FileIDLength {
		return nil, formatError("文件 ID 格式不对")
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

// Keys 是一份文件派生出的两把密钥。
type Keys struct {
	File cipher.AEAD
	Meta cipher.AEAD
}

// DeriveKeys 用主密钥 K 与 fileId 派生文件密钥与元数据密钥。
func DeriveKeys(masterKey []byte, fileID string) (*Keys, error) {
	if len(masterKey) != MasterKeyLength {
		return nil, formatError("主密钥必须是 32 字节")
	}
	salt, err := FileIDBytes(fileID)
	if err != nil {
		return nil, err
	}
	fileKey, err := hkdf.Key(sha256.New, masterKey, salt, protocolVersion+"/file", 32)
	if err != nil {
		return nil, err
	}
	metaKey, err := hkdf.Key(sha256.New, masterKey, salt, protocolVersion+"/meta", 32)
	if err != nil {
		return nil, err
	}
	fileAEAD, err := newAEAD(fileKey)
	if err != nil {
		return nil, err
	}
	metaAEAD, err := newAEAD(metaKey)
	if err != nil {
		return nil, err
	}
	return &Keys{File: fileAEAD, Meta: metaAEAD}, nil
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// ChunkNonce 是第 index 块的 nonce：4 字节零 + 8 字节大端序号。
// 用定长数组返回，避免每块都分配。
func ChunkNonce(index int) [nonceLength]byte {
	var nonce [nonceLength]byte
	binary.BigEndian.PutUint32(nonce[4:8], uint32(index>>32))
	binary.BigEndian.PutUint32(nonce[8:12], uint32(index))
	return nonce
}

// aadPrefix 把 AAD 的固定前缀（版本 ‖ fileId）编码进定长数组，
// 每块只需要改末尾 5 个字节。返回数组而不是切片，调用者可以放在栈上。
type aad [len(protocolVersion) + FileIDLength + 5]byte

// chunkAAD 组装第 index 块的附加数据。
func chunkAAD(fileID string, index int, last bool) aad {
	var out aad
	copy(out[:], protocolVersion)
	if id, err := FileIDBytes(fileID); err == nil {
		copy(out[len(protocolVersion):], id)
	}
	binary.BigEndian.PutUint32(out[len(protocolVersion)+FileIDLength:], uint32(index))
	if last {
		out[len(out)-1] = 1
	}
	return out
}

func metaAAD(fileID string) []byte {
	id, _ := FileIDBytes(fileID)
	out := make([]byte, 0, len(protocolVersion)+FileIDLength)
	out = append(out, protocolVersion...)
	return append(out, id...)
}

// SealChunk 加密一块明文；last 表示这是最后一块。
//
// 返回值是新分配的切片。批量加密时用 SealChunkTo 复用缓冲区，省掉每块 8 MiB 的分配。
func (k *Keys) SealChunk(fileID string, index int, last bool, plain []byte) ([]byte, error) {
	return k.SealChunkTo(nil, fileID, index, last, plain)
}

// SealChunkTo 加密到 dst（容量不足时自动扩容）。
// dst 可以反复复用：返回的是 dst 的前 len(plain)+16 字节。
func (k *Keys) SealChunkTo(dst []byte, fileID string, index int, last bool, plain []byte) ([]byte, error) {
	if len(plain) > ChunkSize {
		return nil, formatError("第 %d 块超过 8 MiB", index)
	}
	if !last && len(plain) != ChunkSize {
		return nil, formatError("非最后一块第 %d 块必须是 8 MiB", index)
	}
	nonce := ChunkNonce(index)
	aad := chunkAAD(fileID, index, last)
	return k.File.Seal(dst[:0], nonce[:], plain, aad[:]), nil
}

// OpenChunk 解密一块密文。任何失败都视为认证失败：调用方必须中止并丢弃已写内容。
func (k *Keys) OpenChunk(fileID string, index int, last bool, sealed []byte) ([]byte, error) {
	return k.OpenChunkTo(nil, fileID, index, last, sealed)
}

// OpenChunkTo 与 OpenChunk 相同，但写入 dst 以复用缓冲区。
func (k *Keys) OpenChunkTo(dst []byte, fileID string, index int, last bool, sealed []byte) ([]byte, error) {
	if len(sealed) < TagLength {
		return nil, formatError("第 %d 块密文短于认证标签", index)
	}
	nonce := ChunkNonce(index)
	aad := chunkAAD(fileID, index, last)
	plain, err := k.File.Open(dst[:0], nonce[:], sealed, aad[:])
	if err != nil {
		return nil, formatError("第 %d 块校验失败，文件可能被篡改或链接不完整", index)
	}
	return plain, nil
}

// Meta 是加密元数据（文件名、大小、类型）。
type Meta struct {
	Version    int    `json:"v"`
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	Type       string `json:"type"`
	ChunkSize  int    `json:"chunkSize"`
	ChunkCount int    `json:"chunkCount"`
}

// NewMeta 组装元数据。
func NewMeta(name string, size int64, contentType string, chunkCount int) Meta {
	return Meta{
		Version:    1,
		Name:       name,
		Size:       size,
		Type:       contentType,
		ChunkSize:  ChunkSize,
		ChunkCount: chunkCount,
	}
}

// SealMeta 加密元数据：固定补到 4 KiB 再加密，base64url 输出 nonce ‖ 密文。
func (k *Keys) SealMeta(fileID string, meta Meta) (string, error) {
	if err := ValidateMeta(meta); err != nil {
		return "", err
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		return "", err
	}
	payload := make([]byte, metaPlainSize)
	if len(raw) > metaPlainSize {
		return "", formatError("元数据过长")
	}
	copy(payload, raw)
	// 尾部补空格：仍是合法 JSON 空白，但长度固定。
	for i := len(raw); i < len(payload); i++ {
		payload[i] = ' '
	}
	nonce, err := RandomBytes(nonceLength)
	if err != nil {
		return "", err
	}
	sealed := k.Meta.Seal(nil, nonce, payload, metaAAD(fileID))
	return Base64URL(append(nonce, sealed...)), nil
}

// OpenMeta 解密元数据。
func (k *Keys) OpenMeta(fileID, sealedB64 string) (Meta, error) {
	var meta Meta
	raw, err := FromBase64URL(sealedB64)
	if err != nil {
		return meta, formatError("元数据不是合法的 base64url")
	}
	if len(raw) <= nonceLength {
		return meta, formatError("元数据密文过短")
	}
	plain, err := k.Meta.Open(nil, raw[:nonceLength], raw[nonceLength:], metaAAD(fileID))
	if err != nil {
		return meta, formatError("元数据解密失败：密钥不对或文件被篡改")
	}
	if err := json.Unmarshal(plain, &meta); err != nil {
		return meta, formatError("元数据不是合法的 JSON")
	}
	if err := ValidateMeta(meta); err != nil {
		return meta, err
	}
	return meta, nil
}

// ValidateMeta 校验元数据字段范围（与 validateFileMeta 一致）。
func ValidateMeta(meta Meta) error {
	if meta.Version != 1 {
		return formatError("不支持的元数据版本")
	}
	if meta.Name == "" || utf8.RuneCountInString(meta.Name) > 255 {
		return formatError("文件名不合法")
	}
	if meta.Size < 0 {
		return formatError("文件大小不合法")
	}
	if len(meta.Type) > 255 {
		return formatError("文件类型不合法")
	}
	if meta.ChunkSize != ChunkSize {
		return formatError("块大小不合法")
	}
	if meta.ChunkCount < 1 {
		return formatError("块数不合法")
	}
	return nil
}

// Wrap 是口令模式的外壳，直接对应服务端 file record 的 wrap 字段。
type Wrap struct {
	KDF        string `json:"kdf"`
	Salt       string `json:"salt"`
	WrappedKey string `json:"wrappedKey"`
}

// keyAAD 是口令外层加密的附加数据。协议里不含 fileId（wrap 在创建文件时提交，此时还没有 fileId）。
func keyAAD() []byte { return []byte(protocolVersion + "/key") }

// WrapMasterKey 用口令包裹主密钥（PBKDF2-HMAC-SHA256 600000 次，口令先做 NFKC 规范化）。
func WrapMasterKey(masterKey []byte, password string) (*Wrap, error) {
	if len(masterKey) != MasterKeyLength {
		return nil, formatError("主密钥必须是 32 字节")
	}
	if utf8.RuneCountInString(password) < 8 {
		return nil, formatError("口令至少 8 位")
	}
	salt, err := RandomBytes(16)
	if err != nil {
		return nil, err
	}
	kek, err := pbkdf2.Key(sha256.New, norm.NFKC.String(password), salt, PBKDF2Iterations, 32)
	if err != nil {
		return nil, err
	}
	aead, err := newAEAD(kek)
	if err != nil {
		return nil, err
	}
	nonce, err := RandomBytes(nonceLength)
	if err != nil {
		return nil, err
	}
	sealed := aead.Seal(nil, nonce, masterKey, keyAAD())
	return &Wrap{
		KDF:        fmt.Sprintf("pbkdf2-sha256-%dk", PBKDF2Iterations/1000),
		Salt:       Base64URL(salt),
		WrappedKey: Base64URL(append(nonce, sealed...)),
	}, nil
}

// UnwrapMasterKey 用口令解出主密钥；口令错误返回 ErrWrongPassword。
func UnwrapMasterKey(w Wrap, password string) ([]byte, error) {
	iterations, err := parseKDF(w.KDF)
	if err != nil {
		return nil, err
	}
	salt, err := FromBase64URL(w.Salt)
	if err != nil {
		return nil, formatError("口令外壳的盐不合法")
	}
	raw, err := FromBase64URL(w.WrappedKey)
	if err != nil {
		return nil, formatError("口令外壳的密文不合法")
	}
	if len(raw) != nonceLength+MasterKeyLength+TagLength {
		return nil, formatError("口令外壳密文长度不对")
	}
	kek, err := pbkdf2.Key(sha256.New, norm.NFKC.String(password), salt, iterations, 32)
	if err != nil {
		return nil, err
	}
	aead, err := newAEAD(kek)
	if err != nil {
		return nil, err
	}
	master, err := aead.Open(nil, raw[:nonceLength], raw[nonceLength:], keyAAD())
	if err != nil {
		return nil, ErrWrongPassword
	}
	return master, nil
}

// kdfPattern 对应 TypeScript 侧的 /^pbkdf2-sha256-([0-9]{3,9})k$/。
var kdfPattern = regexp.MustCompile(`^pbkdf2-sha256-([0-9]{3,9})k$`)

func parseKDF(kdf string) (int, error) {
	match := kdfPattern.FindStringSubmatch(kdf)
	if match == nil {
		return 0, formatError("不支持的口令派生算法 %q", kdf)
	}
	thousands, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, formatError("不支持的口令派生算法 %q", kdf)
	}
	iterations := thousands * 1000
	if iterations < PBKDF2Iterations {
		return 0, formatError("口令派生迭代次数过低")
	}
	return iterations, nil
}
