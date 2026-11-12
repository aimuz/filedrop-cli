package main

import (
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
	fs.BoolVar(&f.password, "password", false, "用口令保护链接")
	fs.IntVar(&f.maxDownloads, "max-downloads", 0, "限制下载次数（0 表示不限）")
	fs.BoolVar(&f.copyLink, "copy", false, "上传成功后把链接复制到剪贴板")
	fs.BoolVar(&f.clipboard, "clipboard", false, "分享剪贴板里的文本")
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
	fs.StringVar(&f.out, "out", ".", "保存目录，默认当前目录；\"-\" 写到标准输出")
	fs.StringVar(&f.password, "password", "", "口令模式的文件用这个口令解密")
	fs.IntVar(&f.concurrency, "concurrency", 0, "同时下载的分块数（0 表示自动，默认 4）")
}

// loginFlags 是 `filedrop login` 的选项。
type loginFlags struct {
	noBrowser bool
}

func bindLoginFlags(fs *pflag.FlagSet, f *loginFlags) {
	fs.BoolVar(&f.noBrowser, "no-browser", false, "只打印链接，不自动打开浏览器")
}
