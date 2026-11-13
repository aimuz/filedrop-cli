package cli

import (
	"strings"
	"testing"

	"github.com/aimuz/filedrop-cli/internal/i18n"
)

// 命令表的文案是运行时取的，切语言后必须跟着变。
func TestCommandTableFollowsLanguage(t *testing.T) {
	original := i18n.Current()
	t.Cleanup(func() { i18n.Set(original) })

	i18n.Set(i18n.EN)
	if got := commandTable()["share"].synopsis; !strings.Contains(got, "<file>") {
		t.Fatalf("英文 synopsis 不对: %q", got)
	}
	i18n.Set(i18n.ZH)
	if got := commandTable()["share"].synopsis; !strings.Contains(got, "<文件>") {
		t.Fatalf("中文 synopsis 不对: %q", got)
	}
}
