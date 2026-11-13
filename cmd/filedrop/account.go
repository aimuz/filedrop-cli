package main

import (
	"fmt"
	"os"

	"github.com/aimuz/filedrop-cli/internal/i18n"
	"github.com/aimuz/filedrop-cli/internal/term"
)

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
	t := i18n.C().CLI
	if len(files) == 0 {
		fmt.Println(t.NoFiles)
		return nil
	}
	fmt.Printf("%-24s %10s  %s\n", t.ColID, t.ColSize, t.ColExpires)
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
		return usageFor("rm")
	}
	c, err := authedClient()
	if err != nil {
		return err
	}
	// 一个删失败不影响后面的：多删几个时，别让第一个错误把剩下的全卡住。
	failed := 0
	for _, id := range ids {
		if err := c.DeleteFile(ctx, id); err != nil {
			failed++
			fmt.Fprintln(os.Stderr, term.Red(id+": ")+err.Error())
			continue
		}
		fmt.Println(fmt.Sprintf(i18n.C().CLI.Deleted, id))
	}
	if failed > 0 {
		return fmt.Errorf(i18n.C().CLI.DeleteFailed, failed)
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
	fmt.Println(fmt.Sprintf(i18n.C().CLI.QuotaLine, term.Bytes(usage.UsedBytes), term.Bytes(usage.QuotaBytes), usage.FileCount))
	return nil
}
