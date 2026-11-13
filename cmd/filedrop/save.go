package main

import (
	"fmt"
	"os"

	"github.com/aimuz/filedrop-cli/internal/filex"
	"github.com/aimuz/filedrop-cli/internal/i18n"
	"github.com/aimuz/filedrop-cli/internal/term"
)

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
	// 先校验选项再联网：参数写错了不该等一轮网络往返才报出来。
	if opts.concurrency < 0 {
		return usage(i18n.C().CLI.ConcurrencyNegative)
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
		value, err := term.ReadPassword(i18n.C().CLI.SavePasswordPrompt)
		if err != nil {
			return "", err
		}
		pass = value
		return value, nil
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
	fmt.Fprintln(os.Stderr, term.Green(fmt.Sprintf(i18n.C().CLI.Saved, result.Path)))
	return nil
}
