package cli

import (
	"github.com/aimuz/filedrop-cli/internal/i18n"
	"github.com/spf13/pflag"
)

// commandSpec 描述一个命令：怎么跑、怎么打用法、有哪些选项。
// run 写成 (*App) 的方法表达式，命令实现就能拿到注入的输出与上下文。
type commandSpec struct {
	run      func(*App, []string) error
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
			run:      (*App).runLogin,
			synopsis: t.SynLogin,
			summary:  t.SumLogin,
			bind:     func(fs *pflag.FlagSet) { bindLoginFlags(fs, &loginFlags{}) },
		},
		"logout": {
			run:      (*App).runLogout,
			synopsis: t.SynLogout,
			summary:  t.SumLogout,
		},
		"whoami": {
			run:      (*App).runWhoami,
			synopsis: t.SynWhoami,
			summary:  t.SumWhoami,
		},
		"share": {
			run:      (*App).runShare,
			synopsis: t.SynShare,
			summary:  t.SumShare,
			bind:     func(fs *pflag.FlagSet) { bindShareFlags(fs, &shareFlags{}) },
		},
		"save": {
			run:      (*App).runSave,
			synopsis: t.SynSave,
			summary:  t.SumSave,
			bind:     func(fs *pflag.FlagSet) { bindSaveFlags(fs, &saveFlags{}) },
		},
		"list": {
			run:      (*App).runList,
			synopsis: t.SynList,
			summary:  t.SumList,
		},
		"rm": {
			run:      (*App).runRm,
			synopsis: t.SynRm,
			summary:  t.SumRm,
		},
		"quota": {
			run:      (*App).runQuota,
			synopsis: t.SynQuota,
			summary:  t.SumQuota,
		},
		"help": {
			run:      (*App).runHelpCommand,
			synopsis: t.SynHelp,
			summary:  t.SumHelp,
		},
		"version": {
			run:      (*App).runVersion,
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
