package filex

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseTarget(t *testing.T) {
	key := "X4pA8CAxewgHYp3vWVsIn6lHxHeEVQ7xPadP-Cpu6xg"
	if _, err := ParseTarget(""); err == nil {
		t.Fatal("空输入必须报错")
	}

	full, err := ParseTarget("https://filedrop.app/d/fQdGQ7ALzoyhsnN01te_GA#" + key)
	if err != nil {
		t.Fatalf("完整链接: %v", err)
	}
	if full.FileID != "fQdGQ7ALzoyhsnN01te_GA" || len(full.Key) != 32 {
		t.Fatalf("完整链接解析不对: %+v", full)
	}

	// 带语言前缀的链接。
	if got, err := ParseTarget("https://filedrop.app/en/d/fQdGQ7ALzoyhsnN01te_GA"); err != nil || got.FileID == "" {
		t.Fatalf("带语言前缀的链接: %v %+v", err, got)
	}

	bare, err := ParseTarget("fQdGQ7ALzoyhsnN01te_GA#" + key)
	if err != nil || bare.FileID != "fQdGQ7ALzoyhsnN01te_GA" || len(bare.Key) != 32 {
		t.Fatalf("裸 ID#密钥: %v %+v", err, bare)
	}

	only, err := ParseTarget("fQdGQ7ALzoyhsnN01te_GA")
	if err != nil || only.FileID == "" || only.Key != nil {
		t.Fatalf("只有 ID: %v %+v", err, only)
	}

	if _, err := ParseTarget("https://filedrop.app/about"); err == nil {
		t.Fatal("非下载链接必须报错")
	}
	if _, err := ParseTarget("fQdGQ7ALzoyhsnN01te_GA#short"); err == nil {
		t.Fatal("密钥长度不对必须报错")
	}
}

func TestUniquePath(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "a.pdf")
	if got := uniquePath(dir, "a.pdf"); got != name {
		t.Fatalf("空目录应返回原名，得到 %s", got)
	}
	if err := os.WriteFile(name, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := uniquePath(dir, "a.pdf"); got != filepath.Join(dir, "a (1).pdf") {
		t.Fatalf("同名文件应改成 a (1).pdf，得到 %s", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "a (1).pdf"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := uniquePath(dir, "a.pdf"); got != filepath.Join(dir, "a (2).pdf") {
		t.Fatalf("应继续递增，得到 %s", got)
	}
}

func TestOpenSink(t *testing.T) {
	dir := t.TempDir()
	out, err := openSink(dir, "../evil.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.path, dir+string(filepath.Separator)) {
		t.Fatalf("文件名里的路径必须被去掉: %s", out.path)
	}
	if err := out.write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := out.close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(out.path)
	if err != nil || !bytes.Equal(raw, []byte("hello")) {
		t.Fatalf("内容不对: %q %v", raw, err)
	}

	// 第二次写同名文件不覆盖。
	second, err := openSink(dir, "evil.txt")
	if err != nil {
		t.Fatal(err)
	}
	second.discard()
	if _, err := os.Stat(second.path); !os.IsNotExist(err) {
		t.Fatal("discard 应删除未完成的文件")
	}

	stdout, err := openSink("-", "x.bin")
	if err != nil || !stdout.stdout {
		t.Fatalf("--out - 应写标准输出: %v", err)
	}
	stdout.discard()
}
