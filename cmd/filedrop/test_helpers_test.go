package main

import (
	"context"
	"io"
	"strings"
)

// testApp 给命令测试一个不碰真实终端与标准输出的 app。
func testApp() *app {
	return newApp(context.Background(), io.Discard, io.Discard, strings.NewReader(""))
}
