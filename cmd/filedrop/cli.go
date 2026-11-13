package main

// CLI 骨架：参数解析、命令表、帮助文本。
// 各命令的实现放在同名文件里（login.go / share.go / save.go / account.go）。

import (
	"errors"
	"fmt"
	"strings"

	"github.com/aimuz/filedrop-cli/internal/i18n"
	"github.com/aimuz/filedrop-cli/internal/term"
	"github.com/spf13/pflag"
)

// usageError 是「参数不对」：打印用法提示，退出码 2（与 getopt 惯例一致）。
type usageError struct{ message string }

func (e *usageError) Error() string { return e.message }

func usagef(format string, args ...any) error {
	return &usageError{message: fmt.Sprintf(format, args...)}
}

// usage 返回不需要格式化的用法错误。
func usage(message string) error {
	return &usageError{message: message}
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
		return usagef(i18n.C().CLI.SeeHelpHint, err)
	}
	return nil
}

// commandSpec 描述一个命令：怎么跑、怎么打用法、有哪些选项。
// bind 为 nil 表示这个命令没有选项。
type commandSpec struct {
	run      func([]string) error
	synopsis string
	// summary 是一两句话的说明，打印在用法行之后。
	summary string
	bind    func(*pflag.FlagSet)
}

// commandTable 既用于分发，也用于打印用法——选项说明只在 bind* 里写一次。
// 用函数而不是包级变量：每次调用都按需构造，没有初始化顺序问题。
func commandTable() map[string]commandSpec {
	t := i18n.C().CLI
	return map[string]commandSpec{
		"login": {
			run:      runLogin,
			synopsis: t.SynLogin,
			summary:  t.SumLogin,
			bind:     func(fs *pflag.FlagSet) { bindLoginFlags(fs, &loginFlags{}) },
		},
		"logout": {
			run:      runLogout,
			synopsis: t.SynLogout,
			summary:  t.SumLogout,
		},
		"whoami": {
			run:      runWhoami,
			synopsis: t.SynWhoami,
			summary:  t.SumWhoami,
		},
		"share": {
			run:      runShare,
			synopsis: t.SynShare,
			summary:  t.SumShare,
			bind:     func(fs *pflag.FlagSet) { bindShareFlags(fs, &shareFlags{}) },
		},
		"save": {
			run:      runSave,
			synopsis: t.SynSave,
			summary:  t.SumSave,
			bind:     func(fs *pflag.FlagSet) { bindSaveFlags(fs, &saveFlags{}) },
		},
		"list": {
			run:      runList,
			synopsis: t.SynList,
			summary:  t.SumList,
		},
		"rm": {
			run:      runRm,
			synopsis: t.SynRm,
			summary:  t.SumRm,
		},
		"quota": {
			run:      runQuota,
			synopsis: t.SynQuota,
			summary:  t.SumQuota,
		},
		"help": {
			run:      runHelpCommand,
			synopsis: t.SynHelp,
			summary:  t.SumHelp,
		},
		"version": {
			run:      runVersion,
			synopsis: t.SynVersion,
			summary:  t.SumVersion,
		},
	}
}

// aliases 是命令的别名，让 `ls` / `delete` 这类习惯用法也能用。
var aliases = map[string]string{
	"ls":     "list",
	"delete": "rm",
}

// lookupCommand 先查别名，再查命令表。
func lookupCommand(name string) (commandSpec, bool) {
	if target, ok := aliases[name]; ok {
		name = target
	}
	spec, ok := commandTable()[name]
	return spec, ok
}

// usageFor 用命令表里的 synopsis 生成「用法」提示，避免各处重复抄一行用法。
func usageFor(name string) error {
	if spec, ok := lookupCommand(name); ok {
		return usagef(i18n.C().CLI.UsageLine, spec.synopsis)
	}
	return usagef("%s: %s", name, i18n.C().CLI.SynHelp)
}

// printCommandUsage 打印单个命令的用法；没登记的命令退回总览。
func printCommandUsage(name string) {
	t := i18n.C().CLI
	spec, ok := lookupCommand(name)
	if !ok {
		fmt.Print(t.Usage)
		return
	}
	fmt.Printf("%s\n", fmt.Sprintf(t.UsageLine, spec.synopsis))
	if spec.summary != "" {
		fmt.Printf("\n%s\n", spec.summary)
	}
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
		fmt.Printf("\n%s\n%s\n", t.OptionsHeader, strings.Join(lines, "\n"))
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
// 这些是命令行语法的一部分，不随语言变化。
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
		return "", usageFor("save")
	case 1:
		return fs.Arg(0), nil
	default:
		return "", usagef(i18n.C().CLI.SaveOnlyOne, fs.Args()[1:])
	}
}

// runHelp 打印某个命令的用法。
func runHelp(name string) error {
	if _, ok := lookupCommand(name); !ok {
		return usagef(i18n.C().CLI.NoSuchCommand, name)
	}
	printCommandUsage(name)
	return errHelp
}

// runHelpCommand 是 `filedrop help` 的实现。
func runHelpCommand(args []string) error {
	fs := newFlags("help")
	if err := parse(fs, args); err != nil {
		return err
	}
	switch fs.NArg() {
	case 0:
		fmt.Print(i18n.C().CLI.Usage)
		return nil
	case 1:
		return runHelp(fs.Arg(0))
	default:
		return usagef(i18n.C().CLI.HelpOnlyOne, fs.Args()[1:])
	}
}

// runVersion 是 `filedrop version` 的实现。
func runVersion(args []string) error {
	if err := parseOnlyArgs("version", args); err != nil {
		return err
	}
	fmt.Println("filedrop", version)
	return nil
}

// version 由构建脚本通过 -ldflags "-X main.version=..." 注入。
var version = "dev"
