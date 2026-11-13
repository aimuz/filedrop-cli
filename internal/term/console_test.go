package term

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestConsoleReadsConsecutiveLines(t *testing.T) {
	var out bytes.Buffer
	console := NewConsole(strings.NewReader("first\nsecond\n"), &out)

	for _, want := range []string{"first", "second"} {
		got, err := console.ReadPassword("prompt: ")
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("读到 %q，期望 %q", got, want)
		}
	}
	if out.String() != "prompt: \nprompt: \n" {
		t.Fatalf("提示输出不对: %q", out.String())
	}
}

func TestConsoleAcceptsLastLineWithoutNewline(t *testing.T) {
	console := NewConsole(strings.NewReader("only"), io.Discard)
	got, err := console.ReadPassword("")
	if err != nil || got != "only" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestConsoleTrimsCRLF(t *testing.T) {
	console := NewConsole(strings.NewReader("value\r\n"), io.Discard)
	got, err := console.ReadPassword("")
	if err != nil || got != "value" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestConsoleEmptyInput(t *testing.T) {
	console := NewConsole(strings.NewReader(""), io.Discard)
	if _, err := console.ReadPassword(""); !errors.Is(err, ErrEmptyInput) {
		t.Fatalf("空输入应返回 ErrEmptyInput，得到 %v", err)
	}
}
