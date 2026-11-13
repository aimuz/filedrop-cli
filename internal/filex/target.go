package filex

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/aimuz/filedrop-cli/internal/crypto"
)

// Target 是从命令行参数解析出来的下载目标。
type Target struct {
	FileID string
	// Key 是链接 # 之后的密钥；为 nil 表示只有 ID（口令模式）。
	Key []byte
}

// ErrMissingKey 表示普通链接丢了 # 之后的密钥：服务端只有密文，谁也解不开。
var ErrMissingKey = errors.New(
	"这个链接缺少 # 之后的密钥，无法解密；请让发送方重新发一次完整链接",
)

var fileIDPattern = regexp.MustCompile(`(?:^|/)d/([A-Za-z0-9_-]{22})(?:[/?#]|$)`)
var bareIDPattern = regexp.MustCompile(`^(?:#)?([A-Za-z0-9_-]{22})(?:#(.*))?$`)

// ParseTarget 解析三种输入：
//   - 完整链接 https://filedrop.app/d/<id>#<key>
//   - <id>#<key>
//   - <id>（口令模式，需要输入口令）
func ParseTarget(raw string) (Target, error) {
	input := strings.TrimSpace(raw)
	if input == "" {
		return Target{}, errors.New("没有指定要下载的链接或 ID")
	}

	if strings.Contains(input, "://") {
		parsed, err := url.Parse(input)
		if err != nil {
			return Target{}, fmt.Errorf("链接格式不对: %w", err)
		}
		match := fileIDPattern.FindStringSubmatch(parsed.Path)
		if match == nil {
			return Target{}, errors.New("链接里找不到文件 ID（应为 /d/<id>）")
		}
		return targetWithKey(match[1], parsed.Fragment)
	}

	match := bareIDPattern.FindStringSubmatch(input)
	if match == nil {
		return Target{}, errors.New("无法识别这个链接或 ID")
	}
	return targetWithKey(match[1], match[2])
}

func targetWithKey(fileID, fragment string) (Target, error) {
	target := Target{FileID: fileID}
	if fragment == "" {
		return target, nil
	}
	key, err := crypto.FromBase64URL(fragment)
	if err != nil || len(key) != crypto.MasterKeyLength {
		return Target{}, errors.New("链接里的密钥格式不对，请确认链接完整")
	}
	target.Key = key
	return target, nil
}
