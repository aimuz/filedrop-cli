package main

import (
	"errors"
	"testing"
)

const testLink = "https://filedrop.app/d/fQdGQ7ALzoyhsnN01te_GA#X4pA8CAxewgHYp3vWVsIn6lHxHeEVQ7xPadP-Cpu6xg"

// parseSave 用与 runSave 相同的方式解析参数，返回目标与选项。
func parseSave(t *testing.T, args []string) (target string, opts saveFlags, err error) {
	t.Helper()
	a := testApp()
	fs := newFlags("save")
	bindSaveFlags(fs, &opts)
	if err := a.parse(fs, args); err != nil {
		return "", opts, err
	}
	target, err = singleTarget(fs)
	return target, opts, err
}

func TestSaveArgs(t *testing.T) {
	cases := []struct {
		name   string
		args   []string
		target string
		out    string
	}{
		{"目标在前", []string{testLink, "--out", "dir"}, testLink, "dir"},
		{"目标在后", []string{"--out", "dir", testLink}, testLink, "dir"},
		{"等号写法", []string{testLink, "--out=dir"}, testLink, "dir"},
		{"只有目标", []string{"fQdGQ7ALzoyhsnN01te_GA"}, "fQdGQ7ALzoyhsnN01te_GA", "."},
		{"-- 之后是目标", []string{"--", "fQdGQ7ALzoyhsnN01te_GA"}, "fQdGQ7ALzoyhsnN01te_GA", "."},
		{"口令与目标", []string{"fQdGQ7ALzoyhsnN01te_GA", "--password", "hunter22"}, "fQdGQ7ALzoyhsnN01te_GA", "."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target, opts, err := parseSave(t, tc.args)
			if err != nil {
				t.Fatalf("解析失败: %v", err)
			}
			if target != tc.target {
				t.Fatalf("target=%q want %q", target, tc.target)
			}
			if opts.out != tc.out {
				t.Fatalf("out=%q want %q", opts.out, tc.out)
			}
		})
	}
}

func TestSaveRejectsUnknownFlag(t *testing.T) {
	// 以前拼错的开关会被当成目标、然后报「用法」；现在直接说清楚是哪个开关。
	_, _, err := parseSave(t, []string{testLink, "--oot", "dir"})
	if _, ok := errors.AsType[*usageError](err); !ok {
		t.Fatalf("拼错的开关必须报错，得到 %v", err)
	}
}

func TestSaveRejectsExtraTargets(t *testing.T) {
	_, _, err := parseSave(t, []string{"aaa", "bbb"})
	if err == nil {
		t.Fatal("多余的参数必须报错")
	}
}

func TestShareArgsInterspersed(t *testing.T) {
	a := testApp()
	fs := newFlags("share")
	var opts shareFlags
	bindShareFlags(fs, &opts)
	if err := a.parse(fs, []string{"a.pdf", "--password", "b.zip", "--max-downloads", "3"}); err != nil {
		t.Fatal(err)
	}
	if got := fs.Args(); len(got) != 2 || got[0] != "a.pdf" || got[1] != "b.zip" {
		t.Fatalf("文件名应原样保留，得到 %q", got)
	}
	if !opts.password || opts.maxDownloads != 3 {
		t.Fatalf("选项没解析对: %+v", opts)
	}
}

func TestRmIDsArePositional(t *testing.T) {
	// 回归测试：ID 曾被 flag 包静默丢掉，命令假装删除成功。
	a := testApp()
	fs := newFlags("rm")
	if err := a.parse(fs, []string{"fQdGQ7ALzoyhsnN01te_GA", "ZQdGQ7ALzoyhsnN01te_GA"}); err != nil {
		t.Fatal(err)
	}
	if got := fs.Args(); len(got) != 2 {
		t.Fatalf("应拿到 2 个 ID，得到 %q", got)
	}

	if err := a.parse(newFlags("rm"), []string{"--force", "x"}); err == nil {
		t.Fatal("rm 没有选项，传开关必须报错")
	}
}
