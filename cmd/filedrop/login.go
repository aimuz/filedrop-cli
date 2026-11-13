package main

import (
	"fmt"
	"os"

	"github.com/aimuz/filedrop-cli/internal/api"
	"github.com/aimuz/filedrop-cli/internal/auth"
	"github.com/aimuz/filedrop-cli/internal/config"
	"github.com/aimuz/filedrop-cli/internal/term"
)

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
