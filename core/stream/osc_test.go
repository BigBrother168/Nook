package stream

import (
	"encoding/base64"
	"reflect"
	"testing"
)

func scan(pieces ...string) [][2]string {
	var got [][2]string
	s := &Scanner{On: func(code, data string) { got = append(got, [2]string{code, data}) }}
	for _, p := range pieces {
		s.Feed([]byte(p))
	}
	return got
}

func TestScannerTerminators(t *testing.T) {
	got := scan("a\x1b]7501;state=working\x07b\x1b]9;hello\x1b\\c\x1b]0;title\x07")
	want := [][2]string{{"7501", "state=working"}, {"9", "hello"}, {"0", "title"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestScannerSplitAcrossReads(t *testing.T) {
	got := scan("x\x1b", "]75", "01;state=bl", "ocked:kind=permission\x1b", "\\y")
	want := [][2]string{{"7501", "state=blocked:kind=permission"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestScannerIgnoresOtherEscapes(t *testing.T) {
	// CSI sequences, colors and UTF-8 text with bytes like 0x9D are not OSC.
	got := scan("\x1b[31m红色\x1b[0m\x1b[?2026h写\x1b[?2026l")
	if len(got) != 0 {
		t.Fatalf("got %q", got)
	}
}

func TestScannerCutShortOSC(t *testing.T) {
	// An OSC left unterminated by a new escape sequence is dropped.
	got := scan("\x1b]7501;state=work\x1b[2J\x1b]7;file://host/c/x\x07")
	want := [][2]string{{"7", "file://host/c/x"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestScannerSkipsHugeOSC(t *testing.T) {
	big := make([]byte, maxOSC+10)
	for i := range big {
		big[i] = 'A'
	}
	got := scan("\x1b]52;c;"+string(big)+"\x07", "\x1b]9;after\x07")
	want := [][2]string{{"9", "after"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %d events, want %q", len(got), want)
	}
}

func TestParseStatus(t *testing.T) {
	msg := base64.StdEncoding.EncodeToString([]byte("允许运行 npm test？"))
	st, ok := ParseStatus("state=blocked:kind=permission:app=claude:msg=" + msg)
	if !ok || st.State != "blocked" || st.Kind != "permission" || st.App != "claude" || st.Message != "允许运行 npm test？" {
		t.Fatalf("got %+v %v", st, ok)
	}
	if _, ok := ParseStatus("?"); ok {
		t.Fatal("a query parsed as a report")
	}
}
