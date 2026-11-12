// Package auth 实现命令行登录：打开网页登录页，然后轮询设备码直到被授权。
package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/aimuz/filedrop-cli/internal/api"
	"github.com/aimuz/filedrop-cli/internal/term"
)

// LoginOptions 控制登录流程。
type LoginOptions struct {
	// ClientName 展示在授权页上，便于用户确认是哪台设备。
	ClientName string
	// NoBrowser 为 true 时只打印链接。
	NoBrowser bool
	// Out 用来打印提示，默认 os.Stdout。
	Out io.Writer
}

// ErrDenied 表示用户在网页上拒绝或票据过期。
var (
	ErrDenied  = errors.New("登录请求被拒绝")
	ErrExpired = errors.New("登录请求已过期，请重新执行 filedrop login")
)

// Login 走完设备码流程，返回长期令牌与账号展示名。
//
// 流程：
//  1. POST /api/auth/device 拿到设备码与授权链接；
//  2. 用户在自己的浏览器里打开链接、选择登录平台并确认（平台选择只发生在网页）；
//  3. 命令行按服务端给的间隔轮询，拿到一次性令牌。
func Login(ctx context.Context, client *api.Client, opts LoginOptions) (token, account string, err error) {
	out := opts.Out
	if out == nil {
		out = io.Discard
	}
	start, err := client.DeviceStart(ctx, opts.ClientName)
	if err != nil {
		return "", "", err
	}

	fmt.Fprintln(out, "在浏览器里打开下面的链接完成登录：")
	fmt.Fprintln(out, "  "+term.Bold(start.AuthURL))
	fmt.Fprintln(out, term.Dim(fmt.Sprintf("设备码 %s，%d 分钟内有效。", start.Code, int(time.Until(time.UnixMilli(start.ExpiresAt)).Minutes()))))
	if !opts.NoBrowser {
		if err := term.OpenBrowser(start.AuthURL); err != nil {
			fmt.Fprintln(out, term.Dim("无法自动打开浏览器，请手动复制上面的链接。"))
		} else {
			fmt.Fprintln(out, term.Dim("已尝试打开浏览器，等待授权…"))
		}
	}

	interval := time.Duration(start.Interval) * time.Second
	if interval < time.Second {
		interval = 2 * time.Second
	}
	deadline := time.UnixMilli(start.ExpiresAt)
	for {
		if time.Now().After(deadline) {
			return "", "", ErrExpired
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", "", ctx.Err()
		case <-timer.C:
		}

		poll, err := client.DevicePoll(ctx, start.Code)
		if err != nil {
			// 瞬时网络错误继续重试；令牌无关。
			if apiErr, ok := errors.AsType[*api.Error](err); ok && apiErr.Status < 500 {
				return "", "", err
			}
			continue
		}
		switch poll.Status {
		case "authorized":
			if poll.Token == "" {
				return "", "", errors.New("服务端没有返回登录令牌")
			}
			session, err := api.New(client.BaseURL, poll.Token).Session(ctx)
			if err != nil {
				return "", "", err
			}
			return poll.Token, session.DisplayName, nil
		case "denied":
			return "", "", ErrDenied
		case "expired":
			return "", "", ErrExpired
		default:
			// pending：继续轮询。
		}
	}
}
