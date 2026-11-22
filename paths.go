package main

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/transfer"
)

// altV is the key with which AI tools on Windows read an image from the
// clipboard themselves (Claude Code, Codex): they take no image from a
// paste there.
const altV = "\x1bv"

// pasteNoText is what Nook types in place of a paste of a clipboard that
// holds no text, and whether it takes the paste: copied files' paths, as
// a drop pastes them, or for an image Alt+V. The terminal would send an
// empty bracketed paste, which ConPTY drops before the program sees it,
// so that Control+V with a screenshot did nothing. A paste with text is
// the terminal's.
func pasteNoText(d transfer.Data, q quoting) (keys string, paste, taken bool) {
	if text, _ := d.Read(transfer.Text); len(text) > 0 {
		return "", false, false
	}
	if files, err := d.Files(); err == nil && len(files) > 0 {
		return quotePaths(q, files), true, true
	}
	if slices.ContainsFunc(d.Formats(), func(f transfer.Format) bool { return strings.HasPrefix(string(f), "image/") }) {
		return altV, false, true
	}
	return "", false, false
}

// quoting is how a shell takes a path as one argument.
type quoting int

const (
	quotePowerShell quoting = iota // 'C:\a b\c', quotes doubled
	quoteCmd                       // "C:\a b\c" when needed
	quoteMSYS                      // '/c/a b/c', as Git Bash names Windows paths
	quoteWSL                       // '/mnt/c/a b/c'
)

// quotePaths returns paths as the shell takes them, separated by spaces,
// with a space after, ready to paste where the cursor is.
func quotePaths(q quoting, paths []string) string {
	var b strings.Builder
	for _, p := range paths {
		b.WriteString(quotePath(q, p))
		b.WriteByte(' ')
	}
	return b.String()
}

func quotePath(q quoting, p string) string {
	switch q {
	case quoteCmd:
		if strings.ContainsAny(p, " \t&()[]{}^=;!'+,`~|<>%") {
			return `"` + p + `"`
		}
		return p
	case quoteMSYS:
		return posixQuote(unixPath(p, "/"))
	case quoteWSL:
		return posixQuote(unixPath(p, "/mnt/"))
	}
	if p != "" && !strings.ContainsFunc(p, func(r rune) bool { return !plainRune(r) }) {
		return p
	}
	// PowerShell's single quotes expand nothing; a quote inside is doubled,
	// the curly ones too, which PowerShell also takes as quotes.
	var b strings.Builder
	b.WriteByte('\'')
	for _, r := range p {
		b.WriteRune(r)
		if r == '\'' || r == '\u2018' || r == '\u2019' || r == '\u201a' || r == '\u201b' {
			b.WriteRune(r)
		}
	}
	b.WriteByte('\'')
	return b.String()
}

// plainRune is a character no shell reads as anything but itself.
func plainRune(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' ||
		strings.ContainsRune(`_-.:\/`, r) || r > 0x7f && r != '\u2018' && r != '\u2019' && r != '\u201a' && r != '\u201b'
}

// unixPath turns C:\a\b into prefix+"c/a/b"; other paths only get slashes.
func unixPath(p, prefix string) string {
	p = strings.ReplaceAll(p, `\`, "/")
	if len(p) >= 2 && p[1] == ':' {
		return prefix + strings.ToLower(p[:1]) + p[2:]
	}
	return p
}

// posixQuote single-quotes s for a POSIX shell when it needs it.
func posixQuote(s string) string {
	if s != "" && !strings.ContainsFunc(s, func(r rune) bool { return !plainRune(r) || r == '\\' }) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
