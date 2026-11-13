package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/aimuz/filedrop-cli/internal/api"
	"github.com/aimuz/filedrop-cli/internal/i18n"
	"github.com/aimuz/filedrop-cli/internal/term"
)

func main() {
	i18n.Set(i18n.Detect())
	ctx, stop := setupSignals()
	defer stop()

	a := newApp(ctx, os.Stdout, os.Stderr, os.Stdin)
	if err := a.run(os.Args[1:]); err != nil {
		exitWith(a.errOut, err)
	}
}

// setupSignals 让第一次 Ctrl-C 只取消当前命令；再按一次恢复默认行为，直接退出。
func setupSignals() (context.Context, func()) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ctx.Done()
		// 取消后注销信号处理：第二次 Ctrl-C 会按系统默认行为立即结束进程。
		stop()
	}()
	return ctx, stop
}

func exitWith(w io.Writer, err error) {
	if errors.Is(err, errHelp) {
		return
	}
	if errors.Is(err, context.Canceled) {
		fmt.Fprintln(w, term.Dim(i18n.C().CLI.Canceled))
		os.Exit(130)
	}
	if usageErr, ok := errors.AsType[*usageError](err); ok {
		fmt.Fprintln(w, term.Red("filedrop:"), usageErr.message)
		os.Exit(2)
	}
	fmt.Fprintln(w, term.Red("filedrop:"), err)
	os.Exit(exitCode(err))
}

func exitCode(err error) int {
	if apiErr, ok := errors.AsType[*api.Error](err); ok && apiErr.Status == 401 {
		return 2
	}
	return 1
}

// version 由构建脚本通过 -ldflags "-X main.version=..." 注入。
var version = "dev"
