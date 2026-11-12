package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/aimuz/filedrop-cli/internal/api"
	"github.com/aimuz/filedrop-cli/internal/auth"
	"github.com/aimuz/filedrop-cli/internal/config"
	"github.com/aimuz/filedrop-cli/internal/filex"
	"github.com/aimuz/filedrop-cli/internal/share"
	"github.com/aimuz/filedrop-cli/internal/term"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, errHelp) {
			return
		}
		if usageErr, ok := errors.AsType[*usageError](err); ok {
			fmt.Fprintln(os.Stderr, term.Red("filedrop:"), usageErr.message)
			os.Exit(2)
		}
		fmt.Fprintln(os.Stderr, term.Red("filedrop:"), err)
		os.Exit(exitCode(err))
	}
}

func exitCode(err error) int {
	if apiErr, ok := errors.AsType[*api.Error](err); ok && apiErr.Status == 401 {
		return 2
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
		// `filedrop help save` 显示单个命令的用法，其余情况显示总览。
		if len(rest) == 0 {
			fmt.Print(usage)
			return nil
		}
		return runHelp(rest[0])
	case "version", "-v", "--version":
		fmt.Println("filedrop", version)
		return nil
	}

	if target, ok := aliases[command]; ok {
		command = target
	}
	spec, ok := commandTable()[command]
	if !ok {
		return usagef("未知命令 %q，运行 filedrop help 查看用法", command)
	}
	return spec.run(rest)
}

// runHelp 打印某个命令的用法。
func runHelp(name string) error {
	if _, ok := commandTable()[name]; !ok {
		return usagef("没有这个命令：%q", name)
	}
	printCommandUsage(name)
	return errHelp
}

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
		return nil, errors.New("尚未登录，先运行 filedrop login")
	}
	return c, nil
}

func runLogin(args []string) error {
	fs := newFlags("login")
	var opts loginFlags
	bindLoginFlags(fs, &opts)
	if err := parse(fs, args); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	token, name, err := auth.Login(ctx, api.NewWithVersion(cfg.APIURL(), "", version), auth.LoginOptions{
		ClientName: fmt.Sprintf("filedrop-cli/%s", version),
		NoBrowser:  opts.noBrowser,
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
	if err := parseOnlyArgs("logout", args); err != nil {
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
	_ = c.Logout(ctx)
	cfg.Token = ""
	cfg.Account = ""
	if err := cfg.Save(); err != nil {
		return err
	}
	fmt.Println("已退出登录。")
	return nil
}

func runWhoami(args []string) error {
	if err := parseOnlyArgs("whoami", args); err != nil {
		return err
	}
	c, err := authedClient()
	if err != nil {
		return err
	}
	info, err := c.Session(ctx)
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
	fs := newFlags("share")
	var opts shareFlags
	bindShareFlags(fs, &opts)
	if err := parse(fs, args); err != nil {
		return err
	}
	files := fs.Args()
	// 先校验选项值，再报缺少输入：`--max-downloads -1` 应该提示次数不对，而不是甩用法。
	if opts.maxDownloads < 0 {
		return usagef("--max-downloads 必须是正整数")
	}
	if len(files) == 0 && !opts.clipboard {
		return usagef("用法：filedrop share <文件>... [--clipboard] [--password] [--max-downloads N]")
	}
	c, cfg, err := client()
	if err != nil {
		return err
	}
	if cfg.Token == "" {
		return errors.New("分享需要登录，先运行 filedrop login（未登录也可以直接在网页上上传）")
	}

	var pass string
	if opts.password {
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

	uploadOpts := share.Options{
		TTLSeconds:   86400,
		MaxDownloads: opts.maxDownloads,
		Password:     pass,
		Progress:     term.NewProgress(os.Stderr),
	}

	// 剪贴板里是文本，只有一个来源；放最前面，和文件一样各自独立。
	var contents []*share.Content
	if opts.clipboard {
		text, err := term.ReadClipboard()
		if err != nil {
			return fmt.Errorf("读取剪贴板失败: %w", err)
		}
		content, err := share.FromText(text, c.BaseURL)
		if err != nil {
			return err
		}
		contents = append(contents, content)
	}
	for _, path := range files {
		content, err := share.FromPath(path, c.BaseURL)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		contents = append(contents, content)
	}

	first := ""
	for _, content := range contents {
		link, err := share.Upload(ctx, c, content, uploadOpts)
		if err != nil {
			return fmt.Errorf("%s: %w", content.Name, err)
		}
		if first == "" {
			first = link
		}
		fmt.Println(link)
	}

	if opts.password {
		fmt.Fprintln(os.Stderr, term.Dim("口令不要和链接放在同一条消息里。"))
	} else {
		fmt.Fprintln(os.Stderr, term.Dim("链接本身就含密钥，直接发给对方即可。"))
	}
	if opts.copyLink && first != "" {
		if err := term.CopyToClipboard(first); err != nil {
			fmt.Fprintln(os.Stderr, term.Dim("复制失败："+err.Error()))
		}
	}
	return nil
}

func runSave(args []string) error {
	fs := newFlags("save")
	var opts saveFlags
	bindSaveFlags(fs, &opts)
	if err := parse(fs, args); err != nil {
		return err
	}
	target, err := singleTarget(fs)
	if err != nil {
		return err
	}

	c, _, err := client()
	if err != nil {
		return err
	}
	pass := opts.password
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

	if opts.concurrency < 0 {
		return usagef("--concurrency 不能是负数")
	}
	result, err := filex.Save(ctx, c, target, filex.SaveOptions{
		OutDir:      opts.out,
		Password:    askPassword,
		Progress:    term.NewProgress(os.Stderr),
		Concurrency: opts.concurrency,
	})
	if err != nil {
		return err
	}
	if opts.out == "-" {
		return nil
	}
	fmt.Fprintln(os.Stderr, term.Green("已保存：")+result.Path)
	return nil
}

func runList(args []string) error {
	if err := parseOnlyArgs("list", args); err != nil {
		return err
	}
	c, err := authedClient()
	if err != nil {
		return err
	}
	files, err := c.ListFiles(ctx)
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
	fs := newFlags("rm")
	if err := parse(fs, args); err != nil {
		return err
	}
	ids := fs.Args()
	if len(ids) == 0 {
		return usagef("用法：filedrop rm <ID>...")
	}
	c, err := authedClient()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := c.DeleteFile(ctx, id); err != nil {
			return fmt.Errorf("%s: %w", id, err)
		}
		fmt.Printf("%s 已删除\n", id)
	}
	return nil
}

func runQuota(args []string) error {
	if err := parseOnlyArgs("quota", args); err != nil {
		return err
	}
	c, err := authedClient()
	if err != nil {
		return err
	}
	usage, err := c.Account(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("%s / %s，共 %d 个文件\n", term.Bytes(usage.UsedBytes), term.Bytes(usage.QuotaBytes), usage.FileCount)
	return nil
}
