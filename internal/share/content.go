package share

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/aimuz/filedrop-cli/internal/i18n"
	"github.com/aimuz/filedrop-cli/internal/term"
)

// MaxFileSize 与网页版一致：单文件 1 GiB。
const MaxFileSize = 1024 * 1024 * 1024

// Content 是要分享的内容：磁盘上的文件，或者来自剪贴板的文本。
//
// Name 是放进加密元数据的文件名（会写进对方的下载目录），Base 是输出链接所用的地址。
type Content struct {
	Name string
	Base string
	open func() (io.ReadCloser, error)
	size int64
}

// FromPath 指向磁盘上的文件。
func FromPath(path, base string) (*Content, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, errors.New(i18n.C().Share.DirectoryNotFile)
	}
	if info.Size() > MaxFileSize {
		return nil, fmt.Errorf(i18n.C().Share.FileTooLarge, term.Bytes(MaxFileSize))
	}
	return &Content{
		Name: filepath.Base(path),
		Base: base,
		size: info.Size(),
		open: func() (io.ReadCloser, error) { return os.Open(path) },
	}, nil
}

// textName 是剪贴板文本的默认文件名；扩展名由内容猜出来，方便对方双击打开。
func textName(text string) string {
	trimmed := strings.TrimSpace(text)
	switch {
	case strings.HasPrefix(trimmed, "{"), strings.HasPrefix(trimmed, "["):
		return "clipboard.json"
	case strings.HasPrefix(trimmed, "<"):
		return "clipboard.html"
	default:
		return "clipboard.txt"
	}
}

// FromText 把一段文本当作文件分享（例如 `filedrop share --clipboard`）。
func FromText(text, base string) (*Content, error) {
	size := int64(len(text))
	if size > MaxFileSize {
		return nil, fmt.Errorf(i18n.C().Share.ContentTooLarge, term.Bytes(MaxFileSize))
	}
	return &Content{
		Name: textName(text),
		Base: base,
		size: size,
		open: func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(text)), nil },
	}, nil
}
