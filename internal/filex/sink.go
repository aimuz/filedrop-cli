package filex

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// sink 是唯一的写出口：文件或标准输出。失败时丢弃已写内容。
type sink struct {
	file   *os.File
	stdout bool
	path   string
	// writer 是标准输出的去处，测试里换成 bytes.Buffer；为空时写 os.Stdout。
	writer io.Writer
}

func (s *sink) write(p []byte) error {
	if s.stdout {
		w := s.writer
		if w == nil {
			w = os.Stdout
		}
		_, err := w.Write(p)
		return err
	}
	_, err := s.file.Write(p)
	return err
}

// writeAt 按偏移写入，供并行下载使用。
//
// WriteAt 不改变文件位置，多个 goroutine 可以同时写不同偏移而互不干扰；
// 这样某个块卡在网络上时，后面的块照样能落盘。标准输出不支持随机写。
func (s *sink) writeAt(p []byte, offset int64) error {
	if s.stdout {
		return errors.New("标准输出不支持并发写入")
	}
	_, err := s.file.WriteAt(p, offset)
	return err
}

// truncate 把文件截到指定长度：并行写会留下空洞（未写区域），
// 最后一块若比前面的块短，也必须显式截断。
func (s *sink) truncate(size int64) error {
	if s.stdout {
		return nil
	}
	return s.file.Truncate(size)
}

func (s *sink) close() error {
	if s.stdout {
		return nil
	}
	return s.file.Close()
}

func (s *sink) discard() {
	if s.stdout || s.file == nil {
		return
	}
	name := s.file.Name()
	s.file.Close()
	os.Remove(name)
}

// openSink 决定写到哪里。默认不覆盖同名文件：自动改成「name (1).ext」。
func openSink(outDir, name string) (*sink, error) {
	if outDir == "-" {
		return &sink{stdout: true, path: "-"}, nil
	}
	dir := outDir
	if dir == "" {
		dir = "."
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	base := filepath.Base(name)
	if base == "" || base == "." || base == ".." {
		base = "download"
	}
	path := uniquePath(dir, base)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	return &sink{file: f, path: path}, nil
}

// uniquePath 在不覆盖的前提下找一个可用文件名。
func uniquePath(dir, base string) string {
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	candidate := filepath.Join(dir, base)
	for i := 1; ; i++ {
		if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate
		}
		candidate = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", stem, i, ext))
	}
}
