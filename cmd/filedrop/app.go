package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/aimuz/filedrop-cli/internal/api"
	"github.com/aimuz/filedrop-cli/internal/config"
	"github.com/aimuz/filedrop-cli/internal/i18n"
	"github.com/aimuz/filedrop-cli/internal/term"
)

// app 收拢一次 CLI 调用的依赖：上下文、输出与终端。
//
// 命令实现挂在它上面，避免各处直接摸 os.Stdout / os.Stderr 或依赖全局 ctx；
// 测试里换成 bytes.Buffer 与 strings.Reader 就能完整跑一遍命令。
type app struct {
	ctx     context.Context
	out     io.Writer
	errOut  io.Writer
	console *term.Console
}

func newApp(ctx context.Context, out, errOut io.Writer, in io.Reader) *app {
	return &app{
		ctx:     ctx,
		out:     out,
		errOut:  errOut,
		console: term.NewConsole(in, errOut),
	}
}

func (a *app) run(args []string) error {
	if len(args) == 0 {
		fmt.Fprint(a.out, i18n.C().CLI.Usage)
		return nil
	}
	command, rest := args[0], args[1:]

	switch command {
	case "-h", "--help":
		return a.runHelpCommand(rest)
	case "-v", "--version":
		return a.runVersion(rest)
	}

	spec, ok := lookupCommand(command)
	if !ok {
		return usagef(i18n.C().CLI.UnknownCommand, command)
	}
	return spec.run(a, rest)
}

// client 创建 API 客户端，同时把本地配置带回来（调用者常要看令牌）。
func (a *app) client() (*api.Client, *config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, err
	}
	return api.NewWithVersion(cfg.APIURL(), cfg.Token, version), cfg, nil
}

// authedClient 返回可用的客户端，调用者必须是已登录的（list / rm / quota）。
func (a *app) authedClient() (*api.Client, error) {
	c, cfg, err := a.client()
	if err != nil {
		return nil, err
	}
	if cfg.Token == "" {
		return nil, errors.New(i18n.C().CLI.NeedLogin)
	}
	return c, nil
}
