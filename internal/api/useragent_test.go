package api

import (
	"runtime"
	"strings"
	"testing"
)

func TestUserAgentCarriesOS(t *testing.T) {
	client := NewWithVersion("https://filedrop.app", "", "1.2.3")
	want := "filedrop-cli/1.2.3 (" + runtime.GOOS + "/" + runtime.GOARCH + ")"
	if client.userAgent != want {
		t.Fatalf("user-agent=%q want %q", client.userAgent, want)
	}
	// 服务端的解析规则要求括号里是 os/arch。
	if !strings.Contains(client.userAgent, "("+runtime.GOOS+"/"+runtime.GOARCH+")") {
		t.Fatalf("user-agent 缺少 os/arch: %s", client.userAgent)
	}
}
