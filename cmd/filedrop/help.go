package main

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/aimuz/filedrop-cli/internal/i18n"
	"github.com/aimuz/filedrop-cli/internal/term"
	"github.com/spf13/pflag"
)

// parse 解析选项；帮助与解析错误都转成给用户看的提示。
func (a *app) parse(fs *pflag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, pflag.ErrHelp) {
			printCommandUsage(a.out, fs.Name())
			return errHelp
		}
		return usagef(i18n.C().CLI.SeeHelpHint, err)
	}
	return nil
}

// parseOnlyArgs 用于没有选项的命令：任何开关都是错误。
func (a *app) parseOnlyArgs(name string, args []string) error {
	return a.parse(newFlags(name), args)
}

// printCommandUsage 打印单个命令的用法；没登记的命令退回总览。
func printCommandUsage(w io.Writer, name string) {
	t := i18n.C().CLI
	spec, ok := lookupCommand(name)
	if !ok {
		fmt.Fprint(w, t.Usage)
		return
	}
	fmt.Fprintf(w, "%s\n", fmt.Sprintf(t.UsageLine, spec.synopsis))
	if spec.summary != "" {
		fmt.Fprintf(w, "\n%s\n", spec.summary)
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
		fmt.Fprintf(w, "\n%s\n%s\n", t.OptionsHeader, strings.Join(lines, "\n"))
	}
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
func (a *app) runHelp(name string) error {
	if _, ok := lookupCommand(name); !ok {
		return usagef(i18n.C().CLI.NoSuchCommand, name)
	}
	printCommandUsage(a.out, name)
	return errHelp
}

// runHelpCommand 是 `filedrop help` 的实现。
func (a *app) runHelpCommand(args []string) error {
	fs := newFlags("help")
	if err := a.parse(fs, args); err != nil {
		return err
	}
	switch fs.NArg() {
	case 0:
		fmt.Fprint(a.out, i18n.C().CLI.Usage)
		return nil
	case 1:
		return a.runHelp(fs.Arg(0))
	default:
		return usagef(i18n.C().CLI.HelpOnlyOne, fs.Args()[1:])
	}
}

// runVersion 是 `filedrop version` 的实现。
func (a *app) runVersion(args []string) error {
	if err := a.parseOnlyArgs("version", args); err != nil {
		return err
	}
	fmt.Fprintln(a.out, "filedrop", version)
	return nil
}
