// Package cli 实现 filedrop 命令行的参数解析、命令分发与各命令。
//
// cmd/filedrop 只保留进程边界（信号、真实标准流、退出码）；
// 其余逻辑放在这里，便于用注入的输入输出做测试。
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"

	"github.com/aimuz/filedrop-cli/internal/api"
	"github.com/aimuz/filedrop-cli/internal/config"
	"github.com/aimuz/filedrop-cli/internal/i18n"
	"github.com/aimuz/filedrop-cli/internal/term"
)

// Version 由构建脚本通过 -ldflags "-X github.com/aimuz/filedrop-cli/internal/cli.Version=..." 注入。
// 没有注入时（例如 go install ...@latest）会退回构建信息里的模块版本。
var Version = "dev"

// resolvedVersion 返回实际展示的版本号。
func resolvedVersion() string {
	if Version != "dev" {
		return Version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return Version
	}
	return versionFromBuildInfo(info)
}

// versionFromBuildInfo 从模块构建信息里取版本。
//
// 从本地源码树构建时 Main.Version 是 v0.0.0-时间-提交 的伪版本，
// 且带有 vcs.revision；展示伪版本只会让人误以为是发布版，这种情况仍用 dev。
// go install module@version 从模块缓存构建，没有 vcs 信息，才是真正的发布版本。
func versionFromBuildInfo(info *debug.BuildInfo) string {
	if info.Main.Version == "" || info.Main.Version == "(devel)" {
		return Version
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			return Version
		}
	}
	return info.Main.Version
}

// Options 控制一次 App 的构造。零值可直接使用：测试里通常只替换 Out/Err。
type Options struct {
	Context context.Context
	In      io.Reader
	Out     io.Writer
	Err     io.Writer
	Version string
}

// App 收拢一次 CLI 调用的依赖：上下文、输出与终端。
//
// 命令实现挂在它上面，避免各处直接摸 os.Stdout / os.Stderr 或依赖全局 ctx；
// 测试里换成 bytes.Buffer 与 strings.Reader 就能完整跑一遍命令。
type App struct {
	ctx     context.Context
	out     io.Writer
	errOut  io.Writer
	console *term.Console
	version string
}

// New 按 Options 创建一个 App。
func New(opts Options) *App {
	if opts.Context == nil {
		opts.Context = context.Background()
	}
	if opts.In == nil {
		opts.In = strings.NewReader("")
	}
	if opts.Out == nil {
		opts.Out = io.Discard
	}
	if opts.Err == nil {
		opts.Err = io.Discard
	}
	if opts.Version == "" {
		opts.Version = resolvedVersion()
	}
	return &App{
		ctx:     opts.Context,
		out:     opts.Out,
		errOut:  opts.Err,
		console: term.NewConsole(opts.In, opts.Err),
		version: opts.Version,
	}
}

// Main 是命令行的进程入口：接上信号、标准流与退出码。
func Main(args []string) int {
	i18n.Set(i18n.Detect())

	ctx, stop := notifySignals()
	defer stop()

	app := New(Options{
		Context: ctx,
		In:      os.Stdin,
		Out:     os.Stdout,
		Err:     os.Stderr,
	})
	return exit(app.errOut, app.Run(args))
}

// notifySignals 让第一次 Ctrl-C 只取消当前命令；再按一次恢复默认行为，直接退出。
func notifySignals() (context.Context, func()) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ctx.Done()
		// 取消后注销信号处理：第二次 Ctrl-C 会按系统默认行为立即结束进程。
		stop()
	}()
	return ctx, stop
}

// Run 分发并执行一条命令。帮助请求也以 error 形式返回，由 exit 识别为正常退出。
func (a *App) Run(args []string) error {
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

// exit 打印错误并返回进程退出码；help 与成功都返回 0。
func exit(w io.Writer, err error) int {
	switch {
	case err == nil, errors.Is(err, errHelp):
		return 0
	case errors.Is(err, context.Canceled):
		fmt.Fprintln(w, term.Dim(i18n.C().CLI.Canceled))
		return 130
	}

	if usageErr, ok := errors.AsType[*usageError](err); ok {
		fmt.Fprintln(w, term.Red("filedrop:"), usageErr.message)
		return 2
	}
	fmt.Fprintln(w, term.Red("filedrop:"), err)
	if apiErr, ok := errors.AsType[*api.Error](err); ok && apiErr.Status == 401 {
		return 2
	}
	return 1
}

// client 创建 API 客户端，同时把本地配置带回来（调用者常要看令牌）。
func (a *App) client() (*api.Client, *config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, err
	}
	return api.NewWithVersion(cfg.APIURL(), cfg.Token, a.version), cfg, nil
}

// authedClient 返回可用的客户端，调用者必须是已登录的（list / rm / quota）。
func (a *App) authedClient() (*api.Client, error) {
	c, cfg, err := a.client()
	if err != nil {
		return nil, err
	}
	if cfg.Token == "" {
		return nil, errors.New(i18n.C().CLI.NeedLogin)
	}
	return c, nil
}
