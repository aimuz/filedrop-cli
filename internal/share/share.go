// Package share 实现「加密后上传到 filedrop」，产出可以直接发给别人的链接。
package share

import (
	"strings"

	"github.com/aimuz/filedrop-cli/internal/crypto"
)

// link 拼出下载链接；wrapped 为 true 时链接里不带主密钥。
//
// base 由调用方传入（API 客户端的 BaseURL），这里不再关心配置来源。
func link(base, fileID string, masterKey []byte, wrapped bool) string {
	url := strings.TrimRight(base, "/") + "/d/" + fileID
	if wrapped {
		return url
	}
	return url + "#" + crypto.Base64URL(masterKey)
}
