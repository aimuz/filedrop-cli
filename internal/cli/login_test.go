package cli

import (
	"bytes"
	"testing"

	"github.com/aimuz/filedrop-cli/internal/i18n"
)

func TestPrintLoginSuccess(t *testing.T) {
	original := i18n.Current()
	t.Cleanup(func() { i18n.Set(original) })

	cases := []struct {
		lang i18n.Language
		want string
	}{
		{i18n.EN, "✓ Authentication complete.\n✓ Logged in as aimuz\n"},
		{i18n.ZH, "✓ 认证完成。\n✓ 已登录：aimuz\n"},
	}
	for _, tc := range cases {
		i18n.Set(tc.lang)
		out := &bytes.Buffer{}
		a := New(Options{Out: out, Version: "dev"})
		a.printLoginSuccess("aimuz")
		if got := out.String(); got != tc.want {
			t.Fatalf("%s 输出不对:\n got %q\nwant %q", tc.lang, got, tc.want)
		}
	}
}
