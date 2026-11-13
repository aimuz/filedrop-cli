// Package term 是终端输出的小工具：颜色、进度、口令输入、剪贴板。
package term

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

var colorEnabled = detectColor()

func detectColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func paint(code, s string) string {
	if !colorEnabled {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

// Red / Green / Dim / Bold 是常用的几种样式。
func Red(s string) string   { return paint("31", s) }
func Green(s string) string { return paint("32", s) }
func Dim(s string) string   { return paint("2", s) }
func Bold(s string) string  { return paint("1", s) }

// isTTY 判断标准输入是否接在终端上。
func isTTY() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// stdin 复用同一个缓冲读取器：每次新建 bufio.Reader 会把管道里多读的数据一起丢掉，
// 连续两次 ReadPassword（例如设置口令时要输入两遍）就会读到 EOF。
var stdin = bufio.NewReader(os.Stdin)

// ErrEmptyInput 表示标准输入已经结束（EOF），没有可读的口令。
var ErrEmptyInput = errors.New("没有读到输入")

// ReadPassword 不带回显地读取一行（终端上）；非终端时退回普通读取，便于管道与测试。
// 按行读取会容忍 CRLF，并允许文件末尾没有换行。
func ReadPassword(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	defer fmt.Fprintln(os.Stderr)
	if !isTTY() {
		return readLine()
	}
	old, err := disableEcho()
	if err != nil {
		// 拿不到终端属性时退回明文输入：功能优先，用户会看到自己输入的内容。
		return readLine()
	}
	defer restoreEcho(old)
	return readLine()
}

func readLine() (string, error) {
	line, err := stdin.ReadString('\n')
	line = strings.TrimRight(line, "\r\n")
	if errors.Is(err, io.EOF) {
		if line != "" {
			return line, nil
		}
		return "", ErrEmptyInput
	}
	return line, err
}

// Progress 在终端里画一行进度；输出不是终端时保持安静，避免污染管道。
type Progress struct {
	out     io.Writer
	enabled bool
	mu      sync.Mutex
	last    time.Time
	label   string
	total   int64
	done    int64
	start   time.Time
}

// NewProgress 创建进度条，写到 stderr。
func NewProgress(out io.Writer) *Progress {
	enabled := false
	if f, ok := out.(*os.File); ok {
		if info, err := f.Stat(); err == nil && info.Mode()&os.ModeCharDevice != 0 {
			enabled = true
		}
	}
	return &Progress{out: out, enabled: enabled, start: time.Now()}
}

// Start 开始一次传输。
func (p *Progress) Start(label string, total int64) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.label, p.total, p.done, p.start = label, total, 0, time.Now()
	p.last = time.Time{}
	p.render()
}

// Update 报告已完成的字节数。
func (p *Progress) Update(done int64) {
	if p == nil || !p.enabled {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.done = done
	p.renderLocked()
}

// Add 按增量推进进度，供并行下载使用（块不是按顺序完成的）。
func (p *Progress) Add(delta int64) {
	if p == nil || !p.enabled || delta <= 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.done += delta
	if p.done > p.total {
		p.done = p.total
	}
	p.renderLocked()
}

// Finish 结束并换行。
func (p *Progress) Finish() {
	if p == nil || !p.enabled {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.done = p.total
	p.renderLocked()
	fmt.Fprintln(p.out)
}

// Abort 结束进度行但不算完成：失败或取消时换行，
// 免得后面的错误提示接在进度条尾巴上。
func (p *Progress) Abort() {
	if p == nil || !p.enabled {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	fmt.Fprintln(p.out)
}

// renderLocked 在持锁状态下渲染，并按 100ms 节流；
// 并行下载会频繁调用，不节流会把终端刷爆。
func (p *Progress) renderLocked() {
	if !p.enabled {
		return
	}
	if time.Since(p.last) < 100*time.Millisecond {
		return
	}
	p.last = time.Now()
	p.render()
}

func (p *Progress) render() {
	if !p.enabled {
		return
	}
	percent := 100.0
	if p.total > 0 {
		percent = float64(p.done) / float64(p.total) * 100
	}
	elapsed := time.Since(p.start).Seconds()
	speed := ""
	if elapsed > 0.3 && p.done > 0 {
		speed = fmt.Sprintf("  %s/s", Bytes(int64(float64(p.done)/elapsed)))
	}
	fmt.Fprintf(p.out, "\r\x1b[2K%s %5.1f%%  %s / %s%s", p.label, percent, Bytes(p.done), Bytes(p.total), speed)
}

// Bytes 把字节数转成人类可读的大小。
func Bytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for value := n / unit; value >= unit && exp < 3; value /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTP"[exp])
}

// Expires 把过期时间格式化成「还有多久」。
func Expires(epochMS int64) string {
	left := time.Until(time.UnixMilli(epochMS))
	if left <= 0 {
		return "已过期"
	}
	left = left.Round(time.Minute)
	switch {
	case left < time.Minute:
		return "不到 1 分钟"
	case left < time.Hour:
		return fmt.Sprintf("还有 %d 分钟", int(left.Minutes()))
	case left < 48*time.Hour:
		return fmt.Sprintf("还有 %d 小时", int(left.Hours()))
	default:
		return fmt.Sprintf("还有 %d 天", int(left.Hours()/24))
	}
}

// OpenBrowser 尽力打开系统浏览器；失败不影响主流程。
func OpenBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// CopyToClipboard 用系统剪贴板命令复制文本。
func CopyToClipboard(text string) error {
	candidates := [][]string{
		{"pbcopy"},
		{"wl-copy"},
		{"xclip", "-selection", "clipboard"},
		{"xsel", "--clipboard", "--input"},
	}
	for _, candidate := range candidates {
		path, err := exec.LookPath(candidate[0])
		if err != nil {
			continue
		}
		cmd := exec.Command(path, candidate[1:]...)
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}
	return fmt.Errorf("找不到可用的剪贴板工具")
}
