package main

// CLI 骨架：参数解析、命令表、帮助文本。
// 具体命令的实现放在 main.go。

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aimuz/filedrop-cli/internal/term"
	"github.com/spf13/pflag"
)

// usageError 是「参数不对」：打印用法提示，退出码 2（与 getopt 惯例一致）。
type usageError struct{ message string }

func (e *usageError) Error() string { return e.message }

func usagef(format string, args ...any) error {
	return &usageError{message: fmt.Sprintf(format, args...)}
}

// errHelp 表示用户主动要了帮助：用法已经打印，正常退出。
var errHelp = errors.New("help requested")

// parse 解析选项；帮助与解析错误都转成给用户看的提示。
func parse(fs *pflag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, pflag.ErrHelp) {
			printCommandUsage(fs.Name())
			return errHelp
		}
		return usagef("%v（运行 filedrop help 查看用法）", err)
	}
	return nil
}

// commandSpec 描述一个命令的用法：帮助里的一行概要，以及它的选项定义。
// bind 为 nil 表示这个命令没有选项。
// commandSpec 描述一个命令：怎么跑、怎么打用法、有哪些选项。
type commandSpec struct {
	run      func([]string) error
	synopsis string
	bind     func(*pflag.FlagSet)
}

// commands 既用于分发，也用于打印用法——选项说明只在 bind* 里写一次。
// 用函数而不是包级变量：命令实现会回头调用 printCommandUsage，包级变量会成初始化环。
func commandTable() map[string]commandSpec {
	return map[string]commandSpec{
		"login":  {run: runLogin, synopsis: "filedrop login [--no-browser]", bind: func(fs *pflag.FlagSet) { bindLoginFlags(fs, &loginFlags{}) }},
		"logout": {run: runLogout, synopsis: "filedrop logout"},
		"whoami": {run: runWhoami, synopsis: "filedrop whoami"},
		"share":  {run: runShare, synopsis: "filedrop share <文件>... | --clipboard [选项]", bind: func(fs *pflag.FlagSet) { bindShareFlags(fs, &shareFlags{}) }},
		"save":   {run: runSave, synopsis: "filedrop save <链接 | ID[#密钥]> [--out DIR] [--password PW]", bind: func(fs *pflag.FlagSet) { bindSaveFlags(fs, &saveFlags{}) }},
		"list":   {run: runList, synopsis: "filedrop list"},
		"rm":     {run: runRm, synopsis: "filedrop rm <ID>..."},
		"quota":  {run: runQuota, synopsis: "filedrop quota"},
	}
}

// aliases 是命令的别名，让 `ls` / `delete` 这类习惯用法也能用。
var aliases = map[string]string{
	"ls":     "list",
	"delete": "rm",
}

// printCommandUsage 打印单个命令的用法；没登记的命令退回总览。
func printCommandUsage(name string) {
	spec, ok := commandTable()[name]
	if !ok {
		fmt.Print(usage)
		return
	}
	fmt.Printf("用法：%s\n", spec.synopsis)
	if spec.bind == nil {
		return
	}
	fs := newFlags(name)
	spec.bind(fs)
	var lines []string
	fs.VisitAll(func(f *pflag.Flag) {
		label := "--" + f.Name
		if f.Value.Type() != "bool" {
			label += " " + valueLabel(f.Name)
		}
		lines = append(lines, "  "+term.PadRight(label, 22)+" "+f.Usage)
	})
	if len(lines) > 0 {
		fmt.Printf("\n选项：\n%s\n", strings.Join(lines, "\n"))
	}
}

// valueLabel 是选项值的占位名，例如 `--out DIR`。
func valueLabel(name string) string {
	if label, ok := valueLabels[name]; ok {
		return label
	}
	return "VALUE"
}

// valueLabels 只列需要值时读起来更清楚的选项。
var valueLabels = map[string]string{
	"out":           "DIR",
	"password":      "PW",
	"max-downloads": "N",
	"concurrency":   "N",
}

// parseOnlyArgs 用于没有选项的命令：任何开关都是错误。
func parseOnlyArgs(name string, args []string) error {
	return parse(newFlags(name), args)
}

// singleTarget 取出唯一的位置参数，多余的直接报错，
// 免得拼错的参数被静默忽略、命令「看起来成功了」。
func singleTarget(fs *pflag.FlagSet) (string, error) {
	switch fs.NArg() {
	case 0:
		return "", usagef("用法：filedrop save <链接 | ID[#密钥]> [--out DIR]")
	case 1:
		return fs.Arg(0), nil
	default:
		return "", usagef("save 只接受一个链接，多余参数：%v", fs.Args()[1:])
	}
}

// version 由构建脚本通过 -ldflags "-X main.version=..." 注入。
var version = "dev"

// ctx 是整条命令的生命周期；以后要接 signal.NotifyContext 时改这一处即可。
var ctx = context.Background()

const usage = `filedrop — 端到端加密的文件传递

用法：
  filedrop login                       登录账号（打开网页授权）
  filedrop whoami                      查看当前登录状态
  filedrop logout                      退出登录
  filedrop share <文件>...             上传并输出加密链接（默认 24 小时、不限次数）
  filedrop share --clipboard           把剪贴板里的文本当成文件分享
  filedrop save <链接 | ID[#密钥]>     下载并解密到当前目录
  filedrop list                        列出账号下的文件（只有 ID、大小与过期时间）
  filedrop rm <ID>...                  删除账号下的文件，立即释放容量
  filedrop quota                       查看账号容量
  filedrop version                     打印版本

命令选项：
  filedrop help <命令>                 查看某个命令的选项，例如 filedrop help save

环境变量：
  FILEDROP_API_URL  API 地址，默认 https://filedrop.app
`
