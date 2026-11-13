package i18n

import (
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode"
)

func TestParse(t *testing.T) {
	cases := []struct {
		code string
		want Language
		ok   bool
	}{
		{"zh", ZH, true},
		{"zh-CN", ZH, true},
		{"zh_CN.UTF-8", ZH, true},
		{"zh-Hans", ZH, true},
		{"en", EN, true},
		{"en_US.UTF-8", EN, true},
		{"en-GB@euro", EN, true},
		{"", "", false},
		{"C", "", false},
		{"POSIX", "", false},
		{"fr_FR", "", false},
	}
	for _, tc := range cases {
		got, ok := Parse(tc.code)
		if got != tc.want || ok != tc.ok {
			t.Errorf("Parse(%q) = %q, %v; want %q, %v", tc.code, got, ok, tc.want, tc.ok)
		}
	}
}

func TestDetect(t *testing.T) {
	clearLocale := func(t *testing.T) {
		t.Helper()
		for _, key := range []string{"FILEDROP_LANG", "LC_ALL", "LC_MESSAGES", "LANG", "LANGUAGE"} {
			t.Setenv(key, "")
		}
	}

	t.Run("FILEDROP_LANG wins", func(t *testing.T) {
		clearLocale(t)
		t.Setenv("FILEDROP_LANG", "en")
		t.Setenv("LANGUAGE", "zh")
		if got := Detect(); got != EN {
			t.Fatalf("FILEDROP_LANG 应优先，得到 %q", got)
		}
	})

	t.Run("invalid FILEDROP_LANG falls through", func(t *testing.T) {
		clearLocale(t)
		t.Setenv("FILEDROP_LANG", "fr")
		t.Setenv("LANG", "zh_CN.UTF-8")
		if got := Detect(); got != ZH {
			t.Fatalf("写错的 FILEDROP_LANG 不应改变系统 locale，得到 %q", got)
		}
	})

	// gettext 的 LANGUAGE 优先于 LC_ALL，即使 LC_ALL 是另一种语言。
	t.Run("LANGUAGE before LC_ALL", func(t *testing.T) {
		clearLocale(t)
		t.Setenv("LANGUAGE", "en_US")
		t.Setenv("LC_ALL", "zh_CN.UTF-8")
		if got := Detect(); got != EN {
			t.Fatalf("LANGUAGE 应优先于 LC_ALL，得到 %q", got)
		}
	})

	// C/POSIX locale 下 gettext 不查翻译，LANGUAGE 也被忽略。
	t.Run("LANGUAGE ignored in C locale", func(t *testing.T) {
		clearLocale(t)
		t.Setenv("LC_ALL", "C")
		t.Setenv("LANGUAGE", "zh")
		if got := Detect(); got != EN {
			t.Fatalf("C locale 应按英文处理，得到 %q", got)
		}
	})

	t.Run("C.UTF-8 is C", func(t *testing.T) {
		clearLocale(t)
		t.Setenv("LANG", "C.UTF-8")
		t.Setenv("LANGUAGE", "zh")
		if got := Detect(); got != EN {
			t.Fatalf("C.UTF-8 应按英文处理，得到 %q", got)
		}
	})

	t.Run("LC_MESSAGES C is C", func(t *testing.T) {
		clearLocale(t)
		t.Setenv("LC_MESSAGES", "C")
		t.Setenv("LANGUAGE", "zh")
		if got := Detect(); got != EN {
			t.Fatalf("LC_MESSAGES=C 应按英文处理，得到 %q", got)
		}
	})

	t.Run("LC_ALL before LANG", func(t *testing.T) {
		clearLocale(t)
		t.Setenv("LC_ALL", "en_GB.UTF-8")
		t.Setenv("LANG", "zh_CN.UTF-8")
		if got := Detect(); got != EN {
			t.Fatalf("LC_ALL 应优先于 LANG，得到 %q", got)
		}
	})

	t.Run("LC_MESSAGES before LANG", func(t *testing.T) {
		clearLocale(t)
		t.Setenv("LC_MESSAGES", "en_US.UTF-8")
		t.Setenv("LANG", "zh_CN.UTF-8")
		if got := Detect(); got != EN {
			t.Fatalf("LC_MESSAGES 应优先于 LANG，得到 %q", got)
		}
	})

	t.Run("LANG", func(t *testing.T) {
		clearLocale(t)
		t.Setenv("LANG", "zh_CN.UTF-8")
		if got := Detect(); got != ZH {
			t.Fatalf("应识别中文，得到 %q", got)
		}
	})

	t.Run("LANGUAGE list", func(t *testing.T) {
		clearLocale(t)
		t.Setenv("LANGUAGE", "fr:en_US")
		if got := Detect(); got != EN {
			t.Fatalf("应使用列表里第一个受支持的语言，得到 %q", got)
		}
	})

	t.Run("unsupported locale falls back to English", func(t *testing.T) {
		clearLocale(t)
		t.Setenv("LANG", "fr_FR.UTF-8")
		if got := Detect(); got != EN {
			t.Fatalf("未支持的语言应回退英文，得到 %q", got)
		}
	})

	t.Run("fallback", func(t *testing.T) {
		clearLocale(t)
		if got := Detect(); got != ZH {
			t.Fatalf("没有 locale 时应兜底中文，得到 %q", got)
		}
	})
}

// 两种语言的每个字段都必须有文案；漏翻会在这里直接暴露。
func TestCatalogsComplete(t *testing.T) {
	for name, catalog := range map[string]*Catalog{"zh": &zhCatalog, "en": &enCatalog} {
		walkCatalog(t, name, "", reflect.ValueOf(catalog).Elem())
	}
}

func walkCatalog(t *testing.T, lang, path string, value reflect.Value) {
	t.Helper()
	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		name := path + "." + value.Type().Field(i).Name
		switch field.Kind() {
		case reflect.String:
			if strings.TrimSpace(field.String()) == "" {
				t.Errorf("%s%s 缺少文案", lang, name)
			}
			if lang == "en" {
				for _, r := range field.String() {
					if unicode.Is(unicode.Han, r) {
						t.Errorf("%s%s 里还有中文: %q", lang, name, field.String())
						break
					}
				}
			}
		case reflect.Struct:
			walkCatalog(t, lang, name, field)
		}
	}
}

// 翻译可以换语序，但占位符必须一一对应，否则运行时会打出 %!s(MISSING)。
func TestCatalogPlaceholdersMatch(t *testing.T) {
	zhText := map[string]string{}
	enText := map[string]string{}
	flattenCatalog(reflect.ValueOf(&zhCatalog).Elem(), "", zhText)
	flattenCatalog(reflect.ValueOf(&enCatalog).Elem(), "", enText)

	verb := regexp.MustCompile(`%[a-zA-Z]`)
	for name, text := range zhText {
		zhVerbs := verb.FindAllString(text, -1)
		enVerbs := verb.FindAllString(enText[name], -1)
		if !slices.Equal(zhVerbs, enVerbs) {
			t.Errorf("%s 的占位符不一致: zh %v, en %v", name, zhVerbs, enVerbs)
		}
	}
}

func flattenCatalog(value reflect.Value, path string, out map[string]string) {
	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		name := path + "." + value.Type().Field(i).Name
		switch field.Kind() {
		case reflect.String:
			out[name] = field.String()
		case reflect.Struct:
			flattenCatalog(field, name, out)
		}
	}
}

func TestDynamicErrorFollowsLanguage(t *testing.T) {
	original := Current()
	t.Cleanup(func() { Set(original) })

	err := NewError(func() string { return C().CLI.Canceled })

	Set(ZH)
	if got := err.Error(); got != "已取消。" {
		t.Fatalf("中文文案不对: %q", got)
	}
	Set(EN)
	if got := err.Error(); got != "Canceled." {
		t.Fatalf("英文文案不对: %q", got)
	}
}

func TestSetIgnoresUnknown(t *testing.T) {
	original := Current()
	t.Cleanup(func() { Set(original) })

	Set(EN)
	Set(Language("fr"))
	if got := Current(); got != EN {
		t.Fatalf("未知语言不应被接受，得到 %q", got)
	}
}

func TestMain(m *testing.M) {
	Set(ZH)
	os.Exit(m.Run())
}
