package crypto

import (
	"crypto/pbkdf2"
	"crypto/sha256"
	"fmt"
	"regexp"
	"strconv"
	"unicode/utf8"

	"github.com/aimuz/filedrop-cli/internal/i18n"
	"golang.org/x/text/unicode/norm"
)

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
		return nil, formatError(i18n.C().Crypto.MasterKeyLength)
	}
	if utf8.RuneCountInString(password) < 8 {
		return nil, formatError(i18n.C().Crypto.PasswordTooShort)
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
		return nil, formatError(i18n.C().Crypto.BadWrapSalt)
	}
	raw, err := FromBase64URL(w.WrappedKey)
	if err != nil {
		return nil, formatError(i18n.C().Crypto.BadWrapCipher)
	}
	if len(raw) != nonceLength+MasterKeyLength+TagLength {
		return nil, formatError(i18n.C().Crypto.BadWrapLength)
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
		return 0, formatErrorf(i18n.C().Crypto.UnsupportedKDF, kdf)
	}
	thousands, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, formatErrorf(i18n.C().Crypto.UnsupportedKDF, kdf)
	}
	iterations := thousands * 1000
	if iterations < PBKDF2Iterations {
		return 0, formatError(i18n.C().Crypto.WeakKDF)
	}
	return iterations, nil
}
