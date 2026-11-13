package term

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aimuz/filedrop-cli/internal/i18n"
)

// ErrEmptyInput 表示输入已经结束（EOF），没有可读的一行。
var ErrEmptyInput = i18n.NewError(func() string { return i18n.C().Term.EmptyInput })

// Console 是口令输入用的终端。读写目标可以替换，测试里不需要真的终端。
//
// 只保留一个 bufio.Reader：每次新建会把管道里多读的数据一起丢掉，
// 连续两次 ReadPassword（例如设置口令要输入两遍）就会读到 EOF。
type Console struct {
	in     io.Reader
	out    io.Writer
	reader *bufio.Reader
}

// NewConsole 让 prompt 写到 out，输入从 in 读。
func NewConsole(in io.Reader, out io.Writer) *Console {
	return &Console{in: in, out: out}
}

// ReadPassword 不带回显地读取一行（终端上）；非终端时退回普通读取，便于管道与测试。
// 按行读取会容忍 CRLF，并允许文件末尾没有换行。
func (c *Console) ReadPassword(prompt string) (string, error) {
	fmt.Fprint(c.out, prompt)
	defer fmt.Fprintln(c.out)

	if !c.canDisableEcho() {
		return c.readLine()
	}
	old, err := disableEcho()
	if err != nil {
		// 拿不到终端属性时退回明文输入：功能优先，用户会看到自己输入的内容。
		return c.readLine()
	}
	defer restoreEcho(old)
	return c.readLine()
}

// canDisableEcho 只在输入确实是标准输入终端时改终端属性：
// disableEcho 操作的是 stdin，换成别的 Reader 时不该动它。
func (c *Console) canDisableEcho() bool {
	file, ok := c.in.(*os.File)
	if !ok || file != os.Stdin {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func (c *Console) readLine() (string, error) {
	if c.reader == nil {
		c.reader = bufio.NewReader(c.in)
	}
	line, err := c.reader.ReadString('\n')
	line = strings.TrimRight(line, "\r\n")
	if errors.Is(err, io.EOF) {
		if line != "" {
			return line, nil
		}
		return "", ErrEmptyInput
	}
	return line, err
}
