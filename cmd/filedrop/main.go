package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/aimuz/filedrop-cli/internal/api"
	"github.com/aimuz/filedrop-cli/internal/auth"
	"github.com/aimuz/filedrop-cli/internal/config"
	"github.com/aimuz/filedrop-cli/internal/filex"
	"github.com/aimuz/filedrop-cli/internal/share"
	"github.com/aimuz/filedrop-cli/internal/term"
)

// version 由构建脚本通过 -ldflags "-X main.version=..." 注入。
var version = "dev"

const usage = `filedrop — 端到端加密的文件传递

用法：
  filedrop login                       登录账号（打开网页授权）
  filedrop whoami                      查看当前登录状态
  filedrop logout                      退出登录
  filedrop share <文件>...             上传并输出加密链接（默认 24 小时、不限次数）
  filedrop save <链接 | ID[#密钥]>     下载并解密到当前目录
  filedrop list                        列出账号下的文件（只有 ID 与大小）
  filedrop rm <ID>...                  删除账号下的文件，立即释放容量
  filedrop quota                       查看账号容量
  filedrop version                     打印版本

share 选项：
  --password        用口令保护链接（链接里不带密钥，对方要输入口令）
  --max-downloads N 限制下载次数
  --copy            上传成功后把第一份链接复制到剪贴板（需要 pbcopy / wl-copy / xclip）

save 选项：
  --out DIR         保存目录，默认当前目录；"--out -" 写到标准输出
  --password PW     口令模式的文件（链接里没有密钥）用这个口令，省略时交互输入

环境变量：
  FILEDROP_API_URL  API 地址，默认 https://filedrop.app
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, term.Red("filedrop:"), err)
		os.Exit(exitCode(err))
	}
}

func exitCode(err error) int {
	var apiErr *api.Error
	if errors.As(err, &apiErr) {
		if apiErr.Status == 401 {
			return 2
		}
	}
	return 1
}

func run(args []string) error {
	if len(args) == 0 {
		fmt.Print(usage)
		return nil
	}
	command, rest := args[0], args[1:]
	switch command {
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	case "version", "-v", "--version":
		fmt.Println("filedrop", version)
		return nil
	case "login":
		return runLogin(rest)
	case "logout":
		return runLogout(rest)
	case "whoami":
		return runWhoami(rest)
	case "share":
		return runShare(rest)
	case "save":
		return runSave(rest)
	case "list", "ls":
		return runList(rest)
	case "rm", "delete":
		return runRm(rest)
	case "quota":
		return runQuota(rest)
	default:
		return fmt.Errorf("未知命令 %q，运行 filedrop help 查看用法", command)
	}
}

func client() (*api.Client, *config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, err
	}
	return api.New(cfg.APIURL(), cfg.Token), cfg, nil
}

func runLogin(args []string) error {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	noBrowser := fs.Bool("no-browser", false, "只打印链接，不自动打开浏览器")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	token, name, err := auth.Login(context.Background(), api.New(cfg.APIURL(), ""), auth.LoginOptions{
		ClientName: fmt.Sprintf("filedrop-cli/%s", version),
		NoBrowser:  *noBrowser,
		Out:        os.Stdout,
	})
	if err != nil {
		return err
	}
	cfg.Token = token
	cfg.Account = name
	if err := cfg.Save(); err != nil {
		return err
	}
	fmt.Println(term.Green("已登录。"))
	return nil
}

func runLogout(args []string) error {
	if err := flag.NewFlagSet("logout", flag.ContinueOnError).Parse(args); err != nil {
		return err
	}
	c, cfg, err := client()
	if err != nil {
		return err
	}
	if cfg.Token == "" {
		fmt.Println("当前没有登录。")
		return nil
	}
	// 服务端撤销失败也照样清本地，避免留下一个用不了又删不掉的令牌。
	_ = c.Logout(context.Background())
	cfg.Token = ""
	cfg.Account = ""
	if err := cfg.Save(); err != nil {
		return err
	}
	fmt.Println("已退出登录。")
	return nil
}

func runWhoami(args []string) error {
	if err := flag.NewFlagSet("whoami", flag.ContinueOnError).Parse(args); err != nil {
		return err
	}
	c, cfg, err := client()
	if err != nil {
		return err
	}
	if cfg.Token == "" {
		return errors.New("尚未登录，先运行 filedrop login")
	}
	info, err := c.Session(context.Background())
	if err != nil {
		return err
	}
	if info.DisplayName != "" {
		fmt.Printf("已登录：%s\n", info.DisplayName)
	} else {
		fmt.Printf("已登录：%s\n", info.UserID)
	}
	return nil
}

func runShare(args []string) error {
	fs := flag.NewFlagSet("share", flag.ContinueOnError)
	password := fs.Bool("password", false, "用口令保护链接")
	maxDownloads := fs.Int("max-downloads", 0, "限制下载次数（0 表示不限）")
	copyLink := fs.Bool("copy", false, "上传成功后把链接复制到剪贴板")
	files, err := parseShareArgs(fs, args)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return errors.New("用法：filedrop share <文件>... [--password] [--max-downloads N]")
	}

	if *maxDownloads < 0 {
		return errors.New("--max-downloads 必须是正整数")
	}
	c, cfg, err := client()
	if err != nil {
		return err
	}
	if cfg.Token == "" {
		return errors.New("分享需要登录，先运行 filedrop login（未登录也可以直接在网页上上传）")
	}

	var pass string
	if *password {
		pass, err = term.ReadPassword("设置口令（至少 8 位）：")
		if err != nil {
			return err
		}
		again, err := term.ReadPassword("再输入一次：")
		if err != nil {
			return err
		}
		if pass != again {
			return errors.New("两次输入的口令不一致")
		}
		if len([]rune(pass)) < 8 {
			return errors.New("口令至少 8 位")
		}
	}

	first := ""
	for _, path := range files {
		link, err := share.File(context.Background(), c, path, share.Options{
			TTLSeconds:   86400,
			MaxDownloads: *maxDownloads,
			Password:     pass,
			Progress:     term.NewProgress(os.Stderr),
		})
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if first == "" {
			first = link
		}
		fmt.Println(link)
	}

	if *password {
		fmt.Fprintln(os.Stderr, term.Dim("口令不要和链接放在同一条消息里。"))
	} else {
		fmt.Fprintln(os.Stderr, term.Dim("链接本身就含密钥，直接发给对方即可。"))
	}
	if *copyLink && first != "" {
		if err := term.CopyToClipboard(first); err != nil {
			fmt.Fprintln(os.Stderr, term.Dim("复制失败："+err.Error()))
		}
	}
	return nil
}

func runSave(args []string) error {
	// 链接里带 #<key>，flag 包会把 "#" 之后当注释；
	// 约定「目标放在最前面，选项放后面」，这样即使不加引号也不会截断。
	var target string
	var rest []string
	for i, arg := range args {
		if arg == "--" {
			if i+1 < len(args) && target == "" {
				target = args[i+1]
			}
			break
		}
		if strings.HasPrefix(arg, "-") {
			rest = append(rest, arg)
			continue
		}
		if target == "" {
			target = arg
			continue
		}
		rest = append(rest, arg)
	}

	fs := flag.NewFlagSet("save", flag.ContinueOnError)
	out := fs.String("out", ".", "保存目录，默认当前目录；\"-\" 写到标准输出")
	password := fs.String("password", "", "口令模式的文件用这个口令解密")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	if target == "" && fs.NArg() == 1 {
		target = fs.Arg(0)
	}
	if target == "" {
		return errors.New("用法：filedrop save <链接 | ID[#密钥]> [-out DIR]")
	}

	c, _, err := client()
	if err != nil {
		return err
	}
	pass := *password

	askPassword := func() (string, error) {
		if pass != "" {
			return pass, nil
		}
		value, err := term.ReadPassword("口令：")
		if err != nil {
			return "", err
		}
		pass = value
		return value, nil
	}

	result, err := filex.Save(context.Background(), c, target, filex.SaveOptions{
		OutDir:   *out,
		Password: askPassword,
		Progress: term.NewProgress(os.Stderr),
	})
	if err != nil {
		return err
	}
	if *out == "-" {
		return nil
	}
	fmt.Fprintln(os.Stderr, term.Green("已保存：")+result.Path)
	return nil
}

func runList(args []string) error {
	if err := flag.NewFlagSet("list", flag.ContinueOnError).Parse(args); err != nil {
		return err
	}
	c, cfg, err := client()
	if err != nil {
		return err
	}
	if cfg.Token == "" {
		return errors.New("尚未登录，先运行 filedrop login")
	}
	files, err := c.ListFiles(context.Background())
	if err != nil {
		return err
	}
	if len(files) == 0 {
		fmt.Println("账号下没有未过期的文件。")
		return nil
	}
	fmt.Printf("%-24s %10s  %s\n", "ID", "SIZE", "EXPIRES")
	for _, file := range files {
		fmt.Printf("%-24s %10s  %s\n", file.FileID, term.Bytes(file.EncryptedSize), term.Expires(file.ExpiresAt))
	}
	return nil
}

func runRm(args []string) error {
	if err := flag.NewFlagSet("rm", flag.ContinueOnError).Parse(args); err != nil {
		return err
	}
	if len(args) == 0 {
		return errors.New("用法：filedrop rm <ID>...")
	}
	c, cfg, err := client()
	if err != nil {
		return err
	}
	if cfg.Token == "" {
		return errors.New("尚未登录，先运行 filedrop login")
	}
	for _, id := range args {
		if err := c.DeleteFile(context.Background(), id); err != nil {
			return fmt.Errorf("%s: %w", id, err)
		}
		fmt.Printf("%s 已删除\n", id)
	}
	return nil
}

func runQuota(args []string) error {
	if err := flag.NewFlagSet("quota", flag.ContinueOnError).Parse(args); err != nil {
		return err
	}
	c, cfg, err := client()
	if err != nil {
		return err
	}
	if cfg.Token == "" {
		return errors.New("尚未登录，先运行 filedrop login")
	}
	usage, err := c.Account(context.Background())
	if err != nil {
		return err
	}
	fmt.Printf("%s / %s，共 %d 个文件\n", term.Bytes(usage.UsedBytes), term.Bytes(usage.QuotaBytes), usage.FileCount)
	return nil
}

// parseShareArgs 允许选项出现在文件名前后。
// flag 包遇到第一个非选项就停止解析，这里把文件之外的参数重新交给它。
func parseShareArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	var files []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return files, nil
		}
		files = append(files, args[0])
		args = args[1:]
	}
}
