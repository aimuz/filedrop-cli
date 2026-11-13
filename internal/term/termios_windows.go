//go:build windows

package term

// Windows 下不处理回显（由控制台自己管理），返回空属性即可。
// 这里不用 syscall.Termios：Windows 的 syscall 包里没有这个类型，
// 用它会让整个 windows 目标编译失败。
func disableEcho() (*struct{}, error) { return nil, nil }

func restoreEcho(*struct{}) {}
