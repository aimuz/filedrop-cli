package main

import (
	"errors"
	"fmt"
)

// usageError 是「参数不对」：打印用法提示，退出码 2（与 getopt 惯例一致）。
type usageError struct{ message string }

func (e *usageError) Error() string { return e.message }

func usagef(format string, args ...any) error {
	return &usageError{message: fmt.Sprintf(format, args...)}
}

// usage 返回不需要格式化的用法错误。
func usage(message string) error {
	return &usageError{message: message}
}

// errHelp 表示用户主动要了帮助：用法已经打印，正常退出。
var errHelp = errors.New("help requested")
