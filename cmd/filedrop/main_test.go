package main

import (
	"os"
	"testing"

	"github.com/aimuz/filedrop-cli/internal/i18n"
)

// 命令测试会断言中文文案；固定语言，避免开发机的 LANG 影响结果。
func TestMain(m *testing.M) {
	i18n.Set(i18n.ZH)
	os.Exit(m.Run())
}
