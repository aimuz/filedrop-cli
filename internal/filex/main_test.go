package filex

import (
	"os"
	"testing"

	"github.com/aimuz/filedrop-cli/internal/i18n"
)

// 测试里的错误文案断言按中文写；固定语言，避免开发者的 LANG 影响结果。
func TestMain(m *testing.M) {
	i18n.Set(i18n.ZH)
	os.Exit(m.Run())
}
