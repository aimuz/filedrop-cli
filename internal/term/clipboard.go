package term

import (
	"bytes"
	"errors"
	"os/exec"
	"runtime"
	"strings"
)

// ErrClipboardEmpty 表示剪贴板里没有文本。
var ErrClipboardEmpty = errors.New("剪贴板是空的（只支持文本内容）")

// ReadClipboard 读取系统剪贴板里的文本。
//
// macOS 用 pbcopy 的搭档 pbpaste；Linux 依次尝试 Wayland 与 X11 的常见工具；
// Windows 走 PowerShell（-Raw 保留换行）。都找不到时返回可读的错误。
func ReadClipboard() (string, error) {
	var candidates [][]string
	switch runtime.GOOS {
	case "darwin":
		candidates = [][]string{{"pbpaste"}}
	case "windows":
		candidates = [][]string{{"powershell", "-NoProfile", "-Command", "Get-Clipboard -Raw"}}
	default:
		candidates = [][]string{
			{"wl-paste", "--no-newline"},
			{"xclip", "-selection", "clipboard", "-o"},
			{"xsel", "--clipboard", "--output"},
		}
	}

	for _, candidate := range candidates {
		path, err := exec.LookPath(candidate[0])
		if err != nil {
			continue
		}
		var stdout, stderr bytes.Buffer
		cmd := exec.Command(path, candidate[1:]...)
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			continue
		}
		text := stdout.String()
		if strings.TrimSpace(text) == "" {
			return "", ErrClipboardEmpty
		}
		return text, nil
	}
	return "", errors.New("找不到可用的剪贴板工具")
}
