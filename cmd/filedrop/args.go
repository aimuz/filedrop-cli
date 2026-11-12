package main

import (
	"errors"
	"flag"
	"strings"
)

// 这里认识的开关：`--out DIR` 需要一个值（可以写成 `--out=DIR`），
// `--password` 是布尔开关。`flag` 包的 `--x`、`-x`、`--x=v` 三种写法都接受，
// 但布尔开关**不能**吞掉后面的参数，否则 `--password <链接>` 会把链接吃掉。
var (
	valueFlags = map[string]bool{"out": true}
	okFlags    = map[string]bool{"out": true, "password": true}
)

// splitTargetAndFlags 把命令行参数拆成「目标」与「选项」。
//
// 下载链接里带 #<key>，而 `flag` 包遇到第一个非选项就停止解析，
// 不加引号时 shell 还会把 # 之后当注释。所以自己扫一遍：
// 听起来像开关、但不在 save 的开关表里的参数，按目标处理（这样输入错了会报「用法」而不是被静默忽略）。
func splitTargetAndFlags(args []string) (target string, flags []string) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			for _, rest := range args[i+1:] {
				if target == "" {
					target = rest
				}
			}
			return target, flags
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			if target == "" {
				target = arg
			}
			continue
		}
		name := strings.TrimLeft(arg, "-")
		hasValue := strings.Contains(name, "=")
		if base := strings.SplitN(name, "=", 2)[0]; !okFlags[base] {
			// 不认识的开关：当作目标，交给后面的解析报错。
			if target == "" {
				target = arg
			}
			continue
		}
		flags = append(flags, arg)
		if !hasValue && valueFlags[name] && i+1 < len(args) {
			flags = append(flags, args[i+1])
			i++
		}
	}
	return target, flags
}

// parseTargetAndFlags 解析 save 的参数：目标是第一个位置参数，选项可以在任何位置。
func parseTargetAndFlags(fs *flag.FlagSet, args []string) (string, error) {
	target, flags := splitTargetAndFlags(args)
	if err := fs.Parse(flags); err != nil {
		return "", err
	}
	if target == "" && fs.NArg() == 1 {
		target = fs.Arg(0)
	}
	if target == "" {
		return "", errors.New("用法：filedrop save <链接 | ID[#密钥]> [-out DIR]")
	}
	return target, nil
}
