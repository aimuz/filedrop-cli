package share

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"

	"github.com/aimuz/filedrop-cli/internal/api"
	"github.com/aimuz/filedrop-cli/internal/term"
)

// uploadError 把服务端的结构化错误翻译成用户能直接照做的提示。
func uploadError(err error) error {
	apiErr, ok := errors.AsType[*api.Error](err)
	if !ok {
		return err
	}
	if apiErr.Code == "limit_exceeded" {
		// 服务端会给出「已用 x of y bytes」；本地化成人能读的提示。
		if used, quota, ok := parseUsage(apiErr.Message); ok {
			return fmt.Errorf(
				"账号容量不足：已用 %s / %s。先运行 filedrop rm <ID> 删除旧文件，或等它们过期",
				term.Bytes(used), term.Bytes(quota),
			)
		}
		return fmt.Errorf("%s（运行 filedrop quota 看容量，或用 filedrop rm 删除旧文件）", apiErr.Message)
	}
	return err
}

var usagePattern = regexp.MustCompile(`account storage full: (\d+) of (\d+) bytes used`)

// parseUsage 解析服务端配额错误里的「已用/上限」字节数。
func parseUsage(message string) (used, quota int64, ok bool) {
	match := usagePattern.FindStringSubmatch(message)
	if match == nil {
		return 0, 0, false
	}
	used, err1 := strconv.ParseInt(match[1], 10, 64)
	quota, err2 := strconv.ParseInt(match[2], 10, 64)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return used, quota, true
}
