package api

import (
	"context"
	"net/http"
	"net/url"
)

// SessionInfo 是 /api/auth/session 的响应。
type SessionInfo struct {
	UserID      string `json:"userId"`
	DisplayName string `json:"displayName"`
}

// Session 返回当前登录账号。
func (c *Client) Session(ctx context.Context) (*SessionInfo, error) {
	var out SessionInfo
	if err := c.do(ctx, http.MethodGet, "/api/auth/session", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Logout 撤销当前令牌。
func (c *Client) Logout(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/api/auth/logout", nil, nil)
}

// DeviceStart 是 POST /api/auth/device 的响应。
type DeviceStart struct {
	Code      string `json:"code"`
	AuthURL   string `json:"authUrl"`
	Interval  int    `json:"interval"`
	ExpiresAt int64  `json:"expiresAt"`
}

// DeviceStart 申请一个登录票据。
func (c *Client) DeviceStart(ctx context.Context, clientName string) (*DeviceStart, error) {
	var out DeviceStart
	if err := c.do(ctx, http.MethodPost, "/api/auth/device", map[string]string{"clientName": clientName}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DevicePoll 是 GET /api/auth/device 的响应。
type DevicePoll struct {
	Status string `json:"status"`
	Token  string `json:"token"`
}

// DevicePoll 轮询登录票据状态。
func (c *Client) DevicePoll(ctx context.Context, code string) (*DevicePoll, error) {
	var out DevicePoll
	path := "/api/auth/device?code=" + url.QueryEscape(code)
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
