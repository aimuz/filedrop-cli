package main

import (
	"testing"
)

func TestSplitTargetAndFlags(t *testing.T) {
	const link = "https://filedrop.app/d/fQdGQ7ALzoyhsnN01te_GA#X4pA8CAxewgHYp3vWVsIn6lHxHeEVQ7xPadP-Cpu6xg"

	cases := []struct {
		name   string
		args   []string
		target string
		flags  []string
	}{
		{"目标在前", []string{link, "--out", "dir"}, link, []string{"--out", "dir"}},
		{"目标在后", []string{"--out", "dir", link}, link, []string{"--out", "dir"}},
		{"等号写法", []string{link, "--out=dir"}, link, []string{"--out=dir"}},
		{"布尔开关不带值", []string{"--password", "--", link}, link, []string{"--password"}},
		{"只有目标", []string{"fQdGQ7ALzoyhsnN01te_GA"}, "fQdGQ7ALzoyhsnN01te_GA", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target, flags := splitTargetAndFlags(tc.args)
			if target != tc.target {
				t.Fatalf("target=%q want %q", target, tc.target)
			}
			if len(flags) != len(tc.flags) {
				t.Fatalf("flags=%q want %q", flags, tc.flags)
			}
			for i := range flags {
				if flags[i] != tc.flags[i] {
					t.Fatalf("flags=%q want %q", flags, tc.flags)
				}
			}
		})
	}
}

func TestSplitTargetAndFlagsAfterDashes(t *testing.T) {
	target, _ := splitTargetAndFlags([]string{"--", "fQdGQ7ALzoyhsnN01te_GA"})
	if target != "fQdGQ7ALzoyhsnN01te_GA" {
		t.Fatalf("-- 之后的参数应作为目标，得到 %q", target)
	}
}
