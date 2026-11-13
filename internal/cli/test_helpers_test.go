package cli

import (
	"context"
	"io"
	"strings"
)

// testApp 给命令测试一个不碰真实终端与标准输出的 App。
func testApp() *App {
	return New(Options{
		Context: context.Background(),
		In:      strings.NewReader(""),
		Out:     io.Discard,
		Err:     io.Discard,
		Version: "dev",
	})
}
