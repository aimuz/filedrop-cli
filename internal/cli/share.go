package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/aimuz/filedrop-cli/internal/i18n"
	"github.com/aimuz/filedrop-cli/internal/share"
	"github.com/aimuz/filedrop-cli/internal/term"
)

// shareTTLSeconds 是命令行上传的保存时长：登录用户固定 24 小时。
// 刻意不做选项：可选值越多，越容易让人以为「长保留」是免费的。
const shareTTLSeconds = 24 * 60 * 60

func (a *App) runShare(args []string) error {
	fs := newFlags("share")
	var opts shareFlags
	bindShareFlags(fs, &opts)
	if err := a.parse(fs, args); err != nil {
		return err
	}
	files := fs.Args()
	// 先校验选项值，再报缺少输入：`--max-downloads -1` 应该提示次数不对，而不是甩用法。
	if opts.maxDownloads < 0 {
		return usage(i18n.C().CLI.MaxDownloadsNegative)
	}
	if len(files) == 0 && !opts.clipboard {
		return usageFor("share")
	}
	c, cfg, err := a.client()
	if err != nil {
		return err
	}
	if cfg.Token == "" {
		return errors.New(i18n.C().CLI.ShareNeedsLogin)
	}

	pass, err := a.askPassword(opts.password)
	if err != nil {
		return err
	}

	contents, err := prepareContents(c.BaseURL, files, opts.clipboard)
	if err != nil {
		return err
	}

	uploadOpts := share.Options{
		TTLSeconds:   shareTTLSeconds,
		MaxDownloads: opts.maxDownloads,
		Password:     pass,
		Progress:     term.NewProgress(a.errOut),
	}

	links := make([]string, 0, len(contents))
	for _, content := range contents {
		link, err := share.Upload(a.ctx, c, content, uploadOpts)
		if err != nil {
			return fmt.Errorf("%s: %w", content.Name, err)
		}
		links = append(links, link)
		fmt.Fprintln(a.out, link)
	}

	if opts.password {
		fmt.Fprintln(a.errOut, term.Dim(i18n.C().CLI.PasswordHint))
	} else {
		fmt.Fprintln(a.errOut, term.Dim(i18n.C().CLI.KeyHint))
	}
	if opts.copyLink {
		if err := term.CopyToClipboard(strings.Join(links, "\n")); err != nil {
			fmt.Fprintln(a.errOut, term.Dim(fmt.Sprintf(i18n.C().CLI.CopyFailed, err)))
		}
	}
	return nil
}

// askPassword 在 --password 时让用户输入两遍口令并校验。
func (a *App) askPassword(enabled bool) (string, error) {
	if !enabled {
		return "", nil
	}
	t := i18n.C().CLI
	pass, err := a.console.ReadPassword(t.SetPasswordPrompt)
	if err != nil {
		return "", err
	}
	again, err := a.console.ReadPassword(t.RepeatPasswordPrompt)
	if err != nil {
		return "", err
	}
	if pass != again {
		return "", errors.New(t.PasswordMismatch)
	}
	if len([]rune(pass)) < 8 {
		return "", errors.New(t.PasswordTooShort)
	}
	return pass, nil
}

// prepareContents 把命令行列出的东西变成待上传的清单。
// 剪贴板里是文本，只有一个来源；放最前面，和文件一样各自独立。
func prepareContents(base string, files []string, clipboard bool) ([]*share.Content, error) {
	var contents []*share.Content
	if clipboard {
		text, err := term.ReadClipboard()
		if err != nil {
			return nil, fmt.Errorf(i18n.C().CLI.ClipboardReadFailed, err)
		}
		content, err := share.FromText(text, base)
		if err != nil {
			return nil, err
		}
		contents = append(contents, content)
	}
	for _, path := range files {
		content, err := share.FromPath(path, base)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		contents = append(contents, content)
	}
	return contents, nil
}
