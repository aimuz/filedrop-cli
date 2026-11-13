package term

import (
	"fmt"
	"time"

	"github.com/aimuz/filedrop-cli/internal/i18n"
)

// Bytes 把字节数转成人类可读的大小。
func Bytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for value := n / unit; value >= unit && exp < 3; value /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTP"[exp])
}

// Expires 把过期时间格式化成「还有多久」。
func Expires(epochMS int64) string {
	t := i18n.C().Term
	left := time.Until(time.UnixMilli(epochMS))
	if left <= 0 {
		return t.Expired
	}
	left = left.Round(time.Minute)
	switch {
	case left < time.Minute:
		return t.LessThanMinute
	case left < time.Hour:
		return fmt.Sprintf(t.MinutesLeft, int(left.Minutes()))
	case left < 48*time.Hour:
		return fmt.Sprintf(t.HoursLeft, int(left.Hours()))
	default:
		return fmt.Sprintf(t.DaysLeft, int(left.Hours()/24))
	}
}
