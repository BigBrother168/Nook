package main

import (
	"testing"

	"github.com/egoist/mygo/transfer"
)

func TestPasteNoText(t *testing.T) {
	png := transfer.Bytes(transfer.PNG, []byte{0x89, 'P', 'N', 'G'})
	files, err := transfer.FileData(`C:\Windows\win.ini`)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name         string
		d            transfer.Data
		keys         string
		paste, taken bool
	}{
		{"text", transfer.TextData("hi"), "", false, false},
		{"text and image, as Office copies", transfer.New(transfer.NewItem(transfer.Bytes(transfer.Text, []byte("hi")), png)), "", false, false},
		{"screenshot", transfer.New(transfer.NewItem(png)), altV, false, true},
		{"copied file", files, `C:\Windows\win.ini `, true, true},
		{"nothing", transfer.Data{}, "", false, false},
	}
	for _, c := range cases {
		keys, paste, taken := pasteNoText(c.d, quotePowerShell)
		if keys != c.keys || paste != c.paste || taken != c.taken {
			t.Errorf("%s: got %q %v %v, want %q %v %v", c.name, keys, paste, taken, c.keys, c.paste, c.taken)
		}
	}
}

func TestQuotePath(t *testing.T) {
	cases := []struct {
		q    quoting
		in   string
		want string
	}{
		{quotePowerShell, `C:\code\app`, `C:\code\app`},
		{quotePowerShell, `C:\My Files\it's.txt`, `'C:\My Files\it''s.txt'`},
		{quotePowerShell, `E:\项目\说明.md`, `E:\项目\说明.md`},
		{quotePowerShell, `C:\a$b`, `'C:\a$b'`},
		{quoteCmd, `C:\code\app`, `C:\code\app`},
		{quoteCmd, `C:\My Files\a&b`, `"C:\My Files\a&b"`},
		{quoteMSYS, `C:\code\app`, `/c/code/app`},
		{quoteMSYS, `D:\My Files\it's`, `'/d/My Files/it'\''s'`},
		{quoteWSL, `E:\Nook\main.go`, `/mnt/e/Nook/main.go`},
	}
	for _, c := range cases {
		if got := quotePath(c.q, c.in); got != c.want {
			t.Errorf("quotePath(%d, %q) = %q, want %q", c.q, c.in, got, c.want)
		}
	}
	if got := quotePaths(quotePowerShell, []string{`C:\a`, `C:\b c`}); got != `C:\a 'C:\b c' ` {
		t.Errorf("quotePaths = %q", got)
	}
}

func TestRunnable(t *testing.T) {
	for _, p := range []string{`E:\x\setup.EXE`, "run.bat", "a.ps1", "b.vbs", "c.js", "d.lnk", "e.cmd", "f.hta"} {
		if !runnable(p) {
			t.Errorf("%s runs when opened", p)
		}
	}
	for _, p := range []string{"a.txt", "b.png", "c.docx", "d.html", "e.go", "f.json", "README"} {
		if runnable(p) {
			t.Errorf("%s does not run", p)
		}
	}
}
