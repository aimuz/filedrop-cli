package cli

import (
	"fmt"

	"github.com/aimuz/filedrop-cli/internal/api"
	"github.com/aimuz/filedrop-cli/internal/auth"
	"github.com/aimuz/filedrop-cli/internal/config"
	"github.com/aimuz/filedrop-cli/internal/i18n"
	"github.com/aimuz/filedrop-cli/internal/term"
)

func (a *App) runLogin(args []string) error {
	fs := newFlags("login")
	var opts loginFlags
	bindLoginFlags(fs, &opts)
	if err := a.parse(fs, args); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	token, name, err := auth.Login(a.ctx, api.NewWithVersion(cfg.APIURL(), "", a.version), auth.LoginOptions{
		ClientName: fmt.Sprintf("filedrop-cli/%s", a.version),
		NoBrowser:  opts.noBrowser,
		Out:        a.out,
	})
	if err != nil {
		return err
	}
	cfg.Token = token
	cfg.Account = name
	if err := cfg.Save(); err != nil {
		return err
	}
	fmt.Fprintln(a.out, term.Green(i18n.C().CLI.LoggedIn))
	return nil
}

func (a *App) runLogout(args []string) error {
	if err := a.parseOnlyArgs("logout", args); err != nil {
		return err
	}
	c, cfg, err := a.client()
	if err != nil {
		return err
	}
	if cfg.Token == "" {
		fmt.Fprintln(a.out, i18n.C().CLI.NotLoggedIn)
		return nil
	}
	// 服务端撤销失败也照样清本地，避免留下一个用不了又删不掉的令牌。
	_ = c.Logout(a.ctx)
	cfg.Token = ""
	cfg.Account = ""
	if err := cfg.Save(); err != nil {
		return err
	}
	fmt.Fprintln(a.out, i18n.C().CLI.LoggedOut)
	return nil
}

func (a *App) runWhoami(args []string) error {
	if err := a.parseOnlyArgs("whoami", args); err != nil {
		return err
	}
	c, err := a.authedClient()
	if err != nil {
		return err
	}
	info, err := c.Session(a.ctx)
	if err != nil {
		return err
	}
	name := info.DisplayName
	if name == "" {
		name = info.UserID
	}
	fmt.Fprintln(a.out, fmt.Sprintf(i18n.C().CLI.CurrentAccount, name))
	return nil
}
