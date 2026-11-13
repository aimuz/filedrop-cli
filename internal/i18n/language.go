// Package i18n 提供命令行界面的多语言文案。
//
// 语言选择遵循 GNU gettext 的约定，只读环境变量，不提供命令行开关：
//
//   - FILEDROP_LANG 是项目自己的覆盖变量，优先级最高；
//   - 否则按 gettext 的消息查找顺序：LANGUAGE > LC_ALL > LC_MESSAGES > LANG；
//   - LANGUAGE 是冒号分隔的优先级列表，且在 C/POSIX locale 下被忽略；
//   - 未识别或未翻译的语言回退英文，完全没有 locale 时回退简体中文。
//
// 新增语言时：在 Language 里加一个代码，在 catalog.go 的字段上补齐该语言的
// Catalog 值，并在 C() 里登记；TestCatalogsComplete 会检查漏翻的字段。
package i18n

import (
	"os"
	"strings"
)

// Language 是界面语言代码，与网页端的 LocaleCode 保持一致。
type Language string

const (
	ZH Language = "zh"
	EN Language = "en"
)

// current 是本次进程使用的语言。CLI 启动时用 Set(Detect()) 确定，
// 之后只读，因而传输 goroutine 里格式化错误信息也是安全的。
// 不在这里读环境变量：库的默认行为保持确定，测试也不会被开发机的 LANG 影响。
var current = ZH

// Current 返回当前语言。
func Current() Language { return current }

// Set 切换语言。只应在命令开始前调用（启动时或测试）。
func Set(lang Language) {
	if lang != ZH && lang != EN {
		return
	}
	current = lang
}

// Parse 把 zh-CN、en_US.UTF-8 这类 locale 代码归一成受支持的语言代码。
func Parse(code string) (Language, bool) {
	code = strings.TrimSpace(code)
	if code == "" {
		return "", false
	}
	lower := strings.ToLower(strings.ReplaceAll(code, "_", "-"))
	if i := strings.IndexAny(lower, ".@"); i >= 0 {
		lower = lower[:i]
	}
	switch {
	case lower == "zh" || strings.HasPrefix(lower, "zh-"):
		return ZH, true
	case lower == "en" || strings.HasPrefix(lower, "en-"):
		return EN, true
	default:
		return "", false
	}
}

// Detect 按 GNU gettext 的语义选择界面语言。
//
// gettext 只关心消息语言：LANGUAGE 是优先级列表，优先于 LC_ALL、
// LC_MESSAGES 和 LANG；但 locale 解析为 C/POSIX 时 LANGUAGE 会被忽略，
// 消息保持英文。FILEDROP_LANG 是本项目的显式覆盖，排在最前面。
func Detect() Language {
	// 项目自己的覆盖变量写错时忽略，继续看系统 locale，避免意外切成英文。
	if value := os.Getenv("FILEDROP_LANG"); value != "" {
		if lang, ok := Parse(value); ok {
			return lang
		}
	}

	// C/POSIX locale 下 gettext 不查翻译目录，LANGUAGE 也不生效。
	if isCLocale(messageLocale()) {
		return EN
	}

	if value := os.Getenv("LANGUAGE"); value != "" {
		for item := range strings.SplitSeq(value, ":") {
			if lang, ok := Parse(item); ok {
				return lang
			}
		}
		return EN
	}

	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if value := os.Getenv(key); value != "" {
			return parseOrEnglish(value)
		}
	}
	return ZH
}

// messageLocale 返回 gettext 用来查消息目录的 locale：
// 第一个非空的 LC_ALL、LC_MESSAGES、LANG。
func messageLocale() string {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return ""
}

// isCLocale 判断 locale 是否是不做翻译的 C/POSIX（含 C.UTF-8）。
func isCLocale(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if i := strings.IndexAny(value, ".@"); i >= 0 {
		value = value[:i]
	}
	switch strings.ToUpper(value) {
	case "C", "POSIX":
		return true
	default:
		return false
	}
}

func parseOrEnglish(value string) Language {
	if lang, ok := Parse(value); ok {
		return lang
	}
	return EN
}

// C 返回当前语言的文案目录。
func C() *Catalog {
	if current == EN {
		return &enCatalog
	}
	return &zhCatalog
}

// DynamicError 的文案在 Error() 调用时才取当前语言。
//
// 包级哨兵错误（例如 ErrWrongPassword）必须保持同一个值，
// 否则 errors.Is / 直接比较会失效；但固定文案又会在设语言之前就冻结。
type DynamicError struct{ Message func() string }

func (e *DynamicError) Error() string { return e.Message() }

// NewError 创建一个延迟取文案的错误，供包级哨兵错误使用。
func NewError(message func() string) error {
	return &DynamicError{Message: message}
}
