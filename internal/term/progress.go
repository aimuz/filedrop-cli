package term

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

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
