package term

import "os"

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
