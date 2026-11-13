package main

import (
	"fmt"

	"github.com/aimuz/filedrop-cli/internal/i18n"
	"github.com/aimuz/filedrop-cli/internal/term"
)

func (a *app) runList(args []string) error {
	if err := a.parseOnlyArgs("list", args); err != nil {
		return err
	}
	c, err := a.authedClient()
	if err != nil {
		return err
	}
	files, err := c.ListFiles(a.ctx)
	if err != nil {
		return err
	}
	t := i18n.C().CLI
	if len(files) == 0 {
		fmt.Fprintln(a.out, t.NoFiles)
		return nil
	}
	fmt.Fprintf(a.out, "%-24s %10s  %s\n", t.ColID, t.ColSize, t.ColExpires)
	for _, file := range files {
		fmt.Fprintf(a.out, "%-24s %10s  %s\n", file.FileID, term.Bytes(file.EncryptedSize), term.Expires(file.ExpiresAt))
	}
	return nil
}

func (a *app) runRm(args []string) error {
	fs := newFlags("rm")
	if err := a.parse(fs, args); err != nil {
		return err
	}
	ids := fs.Args()
	if len(ids) == 0 {
		return usageFor("rm")
	}
	c, err := a.authedClient()
	if err != nil {
		return err
	}
	// 一个删失败不影响后面的：多删几个时，别让第一个错误把剩下的全卡住。
	failed := 0
	for _, id := range ids {
		if err := c.DeleteFile(a.ctx, id); err != nil {
			failed++
			fmt.Fprintln(a.errOut, term.Red(id+": ")+err.Error())
			continue
		}
		fmt.Fprintln(a.out, fmt.Sprintf(i18n.C().CLI.Deleted, id))
	}
	if failed > 0 {
		return fmt.Errorf(i18n.C().CLI.DeleteFailed, failed)
	}
	return nil
}

func (a *app) runQuota(args []string) error {
	if err := a.parseOnlyArgs("quota", args); err != nil {
		return err
	}
	c, err := a.authedClient()
	if err != nil {
		return err
	}
	info, err := c.Account(a.ctx)
	if err != nil {
		return err
	}
	fmt.Fprintln(a.out, fmt.Sprintf(i18n.C().CLI.QuotaLine, term.Bytes(info.UsedBytes), term.Bytes(info.QuotaBytes), info.FileCount))
	return nil
}
