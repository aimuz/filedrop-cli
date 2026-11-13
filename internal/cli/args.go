package cli

import (
	"github.com/aimuz/filedrop-cli/internal/i18n"
	"github.com/spf13/pflag"
)

// newFlags 创建一个支持「选项与参数交错」的 FlagSet。
//
// pflag 与标准库 flag 最大的区别就在这里：`share a.pdf b.zip --password`
// 和 `--password a.pdf b.zip` 都能解析，不需要自己实现「重新交给 flag」的循环。
// 默认遇到不认识的开关直接报错，避免拼错（例如 `--oot`）被当成参数静默忽略。
func newFlags(name string) *pflag.FlagSet {
	fs := pflag.NewFlagSet(name, pflag.ContinueOnError)
	// 自己打印用法；不让 pflag 在出错时往 stderr 里灌一大段 flag 列表。
	fs.Usage = func() {}
	fs.SortFlags = false
	return fs
}

// shareFlags 是 `filedrop share` 的选项。
type shareFlags struct {
	password     bool
	maxDownloads int
	copyLink     bool
	clipboard    bool
}

func bindShareFlags(fs *pflag.FlagSet, f *shareFlags) {
	t := i18n.C().CLI
	fs.BoolVar(&f.password, "password", false, t.FlagPassword)
	fs.IntVar(&f.maxDownloads, "max-downloads", 0, t.FlagMaxDownloads)
	fs.BoolVar(&f.copyLink, "copy", false, t.FlagCopy)
	fs.BoolVar(&f.clipboard, "clipboard", false, t.FlagClipboard)
}

// saveFlags 是 `filedrop save` 的选项。
//
// --password 是带值的字符串选项（`--password PW`）。注意 `--password` 后面如果
// 直接跟的就是链接，链接会被当成口令——口令和链接要分开写。
type saveFlags struct {
	out         string
	password    string
	concurrency int
}

func bindSaveFlags(fs *pflag.FlagSet, f *saveFlags) {
	t := i18n.C().CLI
	fs.StringVar(&f.out, "out", ".", t.FlagOut)
	fs.StringVar(&f.password, "password", "", t.FlagSavePassword)
	fs.IntVar(&f.concurrency, "concurrency", 0, t.FlagConcurrency)
}

// loginFlags 是 `filedrop login` 的选项。
type loginFlags struct {
	noBrowser bool
}

func bindLoginFlags(fs *pflag.FlagSet, f *loginFlags) {
	fs.BoolVar(&f.noBrowser, "no-browser", false, i18n.C().CLI.FlagNoBrowser)
}
