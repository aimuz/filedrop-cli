package term

import (
	"testing"
)

func TestWidth(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"abc", 3},
		{"--password", 10},
		{"—", 1},       // 破折号是「宽度不定」，按 1 列处理
		{"－", 2},       // 全角连字符
		{"限制下载", 8},    // 4 个汉字
		{"a中b", 4},     // 混排
		{"e\u0301", 1}, // e + 组合重音符
	}
	for _, tc := range cases {
		if got := width(tc.input); got != tc.want {
			t.Errorf("width(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestPadRight(t *testing.T) {
	// 中文按 2 列算：补齐后两者显示宽度相同，才能在终端里对齐。
	a := PadRight("--out 目录", 22)
	b := PadRight("--copy", 22)
	if width(a) != width(b) || width(a) != 22 {
		t.Fatalf("对齐失败: %q=%d, %q=%d", a, width(a), b, width(b))
	}
	// 已经超长时不截断。
	if got := PadRight("abcdef", 3); got != "abcdef" {
		t.Fatalf("超长不应截断，得到 %q", got)
	}
}
