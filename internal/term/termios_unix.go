//go:build !windows

package term

import (
	"syscall"
	"unsafe"
)

// disableEcho 关闭终端回显，返回原来的属性以便恢复。
func disableEcho() (*syscall.Termios, error) {
	fd := int(syscall.Stdin)
	old, err := getTermios(fd)
	if err != nil {
		return nil, err
	}
	next := *old
	next.Lflag &^= syscall.ECHO
	next.Lflag |= syscall.ICANON | syscall.ISIG
	next.Iflag |= syscall.ICRNL
	if err := setTermios(fd, &next); err != nil {
		return nil, err
	}
	return old, nil
}

func restoreEcho(old *syscall.Termios) {
	if old == nil {
		return
	}
	_ = setTermios(int(syscall.Stdin), old)
}

func getTermios(fd int) (*syscall.Termios, error) {
	var t syscall.Termios
	if _, _, errno := syscall.Syscall6(
		syscall.SYS_IOCTL,
		uintptr(fd),
		uintptr(ioctlReadTermios),
		uintptr(unsafe.Pointer(&t)),
		0, 0, 0,
	); errno != 0 {
		return nil, errno
	}
	return &t, nil
}

func setTermios(fd int, t *syscall.Termios) error {
	if _, _, errno := syscall.Syscall6(
		syscall.SYS_IOCTL,
		uintptr(fd),
		uintptr(ioctlWriteTermios),
		uintptr(unsafe.Pointer(t)),
		0, 0, 0,
	); errno != 0 {
		return errno
	}
	return nil
}
