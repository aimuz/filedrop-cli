package term

import (
	"strings"
	"unicode"
)

// width 估算字符串在终端里占的列数：中日韩等全角字符算 2 列，组合符号不占列。
//
// 不引入完整的 Unicode 宽度表：帮助里对齐只需要区分「宽字符」与「窄字符」，
// 用 unicode 的类别判断已经足够，也不增加依赖。
func width(s string) int {
	columns := 0
	for _, r := range s {
		switch {
		case unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf):
			// 组合符号与零宽字符。
		case isWide(r):
			columns += 2
		default:
			columns++
		}
	}
	return columns
}

// isWide 判断常见东亚宽字符所在的区段（与 EastAsianWidth=W/F 的主要范围一致）。
func isWide(r rune) bool {
	return r >= 0x1100 && (r <= 0x115F || // Hangul Jamo
		r == 0x2329 || r == 0x232A ||
		(r >= 0x2E80 && r <= 0xA4CF && r != 0x303F) || // CJK 部首、汉字、假名……
		(r >= 0xAC00 && r <= 0xD7A3) || // Hangul 音节
		(r >= 0xF900 && r <= 0xFAFF) || // CJK 兼容汉字
		(r >= 0xFE30 && r <= 0xFE6F) || // CJK 兼容形式
		(r >= 0xFF00 && r <= 0xFF60) || // 全角 ASCII
		(r >= 0xFFE0 && r <= 0xFFE6) ||
		(r >= 0x20000 && r <= 0x3FFFD)) // CJK 扩展 B 及以后
}

// PadRight 在右侧补空格，使整个字符串占满 columns 列。
func PadRight(s string, columns int) string {
	if padding := columns - width(s); padding > 0 {
		return s + strings.Repeat(" ", padding)
	}
	return s
}
