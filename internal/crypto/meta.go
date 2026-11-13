package crypto

import (
	"encoding/json"
	"unicode/utf8"

	"github.com/aimuz/filedrop-cli/internal/i18n"
)

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

func metaAAD(fileID string) []byte {
	id, _ := FileIDBytes(fileID)
	out := make([]byte, 0, len(protocolVersion)+FileIDLength)
	out = append(out, protocolVersion...)
	return append(out, id...)
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
		return "", formatError(i18n.C().Crypto.MetaTooLong)
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
		return meta, formatError(i18n.C().Crypto.MetaBadBase64)
	}
	if len(raw) <= nonceLength {
		return meta, formatError(i18n.C().Crypto.MetaTooShort)
	}
	plain, err := k.Meta.Open(nil, raw[:nonceLength], raw[nonceLength:], metaAAD(fileID))
	if err != nil {
		return meta, formatError(i18n.C().Crypto.MetaAuthFailed)
	}
	if err := json.Unmarshal(plain, &meta); err != nil {
		return meta, formatError(i18n.C().Crypto.MetaBadJSON)
	}
	if err := ValidateMeta(meta); err != nil {
		return meta, err
	}
	return meta, nil
}

// ValidateMeta 校验元数据字段范围（与 validateFileMeta 一致）。
func ValidateMeta(meta Meta) error {
	if meta.Version != 1 {
		return formatError(i18n.C().Crypto.MetaVersion)
	}
	if meta.Name == "" || utf8.RuneCountInString(meta.Name) > 255 {
		return formatError(i18n.C().Crypto.BadName)
	}
	if meta.Size < 0 {
		return formatError(i18n.C().Crypto.BadSize)
	}
	if len(meta.Type) > 255 {
		return formatError(i18n.C().Crypto.BadType)
	}
	if meta.ChunkSize != ChunkSize {
		return formatError(i18n.C().Crypto.BadChunkSize)
	}
	if meta.ChunkCount < 1 {
		return formatError(i18n.C().Crypto.BadChunkCount)
	}
	return nil
}
