package stream

import (
	"bytes"
	"regexp"
	"strings"
)

// localURL is an address of this computer a program prints, as a
// development server does: http://localhost:5173/.
var localURL = regexp.MustCompile(`https?://(?:localhost|127\.0\.0\.1|0\.0\.0\.0|\[::1?\])(?::\d{2,5})?(?:/[^\s\x00-\x1f"'<>` + "`" + `\\]*)?`)

// URLFinder finds the local addresses in a terminal's output, each once,
// as programs print them in pieces.
type URLFinder struct {
	// On gets each address found, as a browser opens it.
	On   func(url string)
	tail []byte
	seen map[string]bool
}

// maxTail is how much of the output the finder keeps for an address
// split between two reads.
const maxTail = 512

// Feed reads output. An address at the end of b waits for what follows,
// which may go on with it.
func (f *URLFinder) Feed(b []byte) {
	buf := append(f.tail, b...)
	f.tail = nil
	if !bytes.Contains(buf, []byte("://")) {
		f.keep(buf)
		return
	}
	end := 0
	for _, m := range localURL.FindAllIndex(buf, -1) {
		if m[1] == len(buf) {
			// It may go on in the next read.
			f.tail = append([]byte(nil), buf[m[0]:]...)
			return
		}
		f.found(string(buf[m[0]:m[1]]))
		end = m[1]
	}
	f.keep(buf[end:])
}

// keep keeps the end of b, which may hold the start of an address.
func (f *URLFinder) keep(b []byte) {
	if len(b) > maxTail {
		b = b[len(b)-maxTail:]
	}
	f.tail = append([]byte(nil), b...)
}

func (f *URLFinder) found(u string) {
	u = NormalizeLocal(u)
	if f.seen == nil {
		f.seen = map[string]bool{}
	}
	if f.seen[u] || f.On == nil {
		return
	}
	f.seen[u] = true
	f.On(u)
}

// NormalizeLocal returns a local address as a browser opens it: 0.0.0.0
// and [::], which servers listen on, as localhost; without the punctuation of the
// sentence it ends.
func NormalizeLocal(u string) string {
	u = strings.TrimRight(u, ".,;:)]}'\"")
	if strings.Count(u, "[") > strings.Count(u, "]") { // the end of [::1]
		u += "]"
	}
	u = strings.Replace(u, "://0.0.0.0", "://localhost", 1)
	u = strings.Replace(u, "://[::]", "://localhost", 1)
	if strings.Count(u, "/") == 3 && strings.HasSuffix(u, "/") {
		u = strings.TrimSuffix(u, "/")
	}
	return u
}
