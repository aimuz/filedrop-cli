//go:build windows

package term

import "syscall"

// Windows 下不处理回显（由控制台自己管理），返回空属性即可。
func disableEcho() (*syscall.Termios, error) { return nil, nil }

func restoreEcho(*syscall.Termios) {}
