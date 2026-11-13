package filex

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/aimuz/filedrop-cli/internal/crypto"
	"github.com/aimuz/filedrop-cli/internal/i18n"
)

// Target 是从命令行参数解析出来的下载目标。
type Target struct {
	FileID string
	// Key 是链接 # 之后的密钥；为 nil 表示只有 ID（口令模式）。
	Key []byte
}

// ErrMissingKey 表示普通链接丢了 # 之后的密钥：服务端只有密文，谁也解不开。
var ErrMissingKey = i18n.NewError(func() string { return i18n.C().Filex.MissingKey })

var fileIDPattern = regexp.MustCompile(`(?:^|/)d/([A-Za-z0-9_-]{22})(?:[/?#]|$)`)
var bareIDPattern = regexp.MustCompile(`^(?:#)?([A-Za-z0-9_-]{22})(?:#(.*))?$`)

// ParseTarget 解析三种输入：
//   - 完整链接 https://filedrop.app/d/<id>#<key>
//   - <id>#<key>
//   - <id>（口令模式，需要输入口令）
func ParseTarget(raw string) (Target, error) {
	input := strings.TrimSpace(raw)
	if input == "" {
		return Target{}, errors.New(i18n.C().Filex.EmptyTarget)
	}

	if strings.Contains(input, "://") {
		parsed, err := url.Parse(input)
		if err != nil {
			return Target{}, fmt.Errorf(i18n.C().Filex.BadURL, err)
		}
		match := fileIDPattern.FindStringSubmatch(parsed.Path)
		if match == nil {
			return Target{}, errors.New(i18n.C().Filex.MissingFileID)
		}
		return targetWithKey(match[1], parsed.Fragment)
	}

	match := bareIDPattern.FindStringSubmatch(input)
	if match == nil {
		return Target{}, errors.New(i18n.C().Filex.UnrecognizedTarget)
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
		return Target{}, errors.New(i18n.C().Filex.BadKey)
	}
	target.Key = key
	return target, nil
}
