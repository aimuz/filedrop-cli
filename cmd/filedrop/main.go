package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/aimuz/filedrop-cli/internal/api"
	"github.com/aimuz/filedrop-cli/internal/config"
	"github.com/aimuz/filedrop-cli/internal/i18n"
	"github.com/aimuz/filedrop-cli/internal/term"
)

// ctx 是整条命令的生命周期。main 里接上 Ctrl-C：取消正在进行的传输，
// 而不是让进程在半路被杀掉、留下写了一半的文件。
var ctx = context.Background()

func main() {
	i18n.Set(i18n.Detect())
	stop := setupSignals()
	defer stop()

	if err := run(os.Args[1:]); err != nil {
		exitWith(err)
	}
}

// setupSignals 让第一次 Ctrl-C 只取消当前命令；再按一次恢复默认行为，直接退出。
func setupSignals() func() {
	notified, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	ctx = notified
	go func() {
		<-notified.Done()
		// 取消后注销信号处理：第二次 Ctrl-C 会按系统默认行为立即结束进程。
		stop()
	}()
	return stop
}

func exitWith(err error) {
	if errors.Is(err, errHelp) {
		return
	}
	if errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, term.Dim(i18n.C().CLI.Canceled))
		os.Exit(130)
	}
	if usageErr, ok := errors.AsType[*usageError](err); ok {
		fmt.Fprintln(os.Stderr, term.Red("filedrop:"), usageErr.message)
		os.Exit(2)
	}
	fmt.Fprintln(os.Stderr, term.Red("filedrop:"), err)
	os.Exit(exitCode(err))
}

func exitCode(err error) int {
	if apiErr, ok := errors.AsType[*api.Error](err); ok && apiErr.Status == 401 {
		return 2
	}
	return 1
}

func run(args []string) error {
	if len(args) == 0 {
		fmt.Print(i18n.C().CLI.Usage)
		return nil
	}
	command, rest := args[0], args[1:]

	switch command {
	case "-h", "--help":
		return runHelpCommand(rest)
	case "-v", "--version":
		return runVersion(rest)
	}

	spec, ok := lookupCommand(command)
	if !ok {
		return usagef(i18n.C().CLI.UnknownCommand, command)
	}
	return spec.run(rest)
}

// client 创建 API 客户端，同时把本地配置带回来（调用者常要看令牌）。
func client() (*api.Client, *config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, err
	}
	return api.NewWithVersion(cfg.APIURL(), cfg.Token, version), cfg, nil
}

// authedClient 返回可用的客户端，调用者必须是已登录的（list / rm / quota）。
func authedClient() (*api.Client, error) {
	c, cfg, err := client()
	if err != nil {
		return nil, err
	}
	if cfg.Token == "" {
		return nil, errors.New(i18n.C().CLI.NeedLogin)
	}
	return c, nil
}
