package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/binary"

	"github.com/aimuz/filedrop-cli/internal/i18n"
)

// Keys 是一份文件派生出的两把密钥。
type Keys struct {
	File cipher.AEAD
	Meta cipher.AEAD
}

// DeriveKeys 用主密钥 K 与 fileId 派生文件密钥与元数据密钥。
func DeriveKeys(masterKey []byte, fileID string) (*Keys, error) {
	if len(masterKey) != MasterKeyLength {
		return nil, formatError(i18n.C().Crypto.MasterKeyLength)
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

// aad 把 AAD 的固定前缀（版本 ‖ fileId）编码进定长数组，
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
		return nil, formatErrorf(i18n.C().Crypto.ChunkTooLarge, index)
	}
	if !last && len(plain) != ChunkSize {
		return nil, formatErrorf(i18n.C().Crypto.ChunkNotFull, index)
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
		return nil, formatErrorf(i18n.C().Crypto.ChunkTooShort, index)
	}
	nonce := ChunkNonce(index)
	aad := chunkAAD(fileID, index, last)
	plain, err := k.File.Open(dst[:0], nonce[:], sealed, aad[:])
	if err != nil {
		return nil, formatErrorf(i18n.C().Crypto.ChunkAuthFailed, index)
	}
	return plain, nil
}
