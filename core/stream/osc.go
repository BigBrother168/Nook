// Package stream reads what programs tell the terminal, in the bytes they
// write: their status (OSC 7501), notifications (OSC 9, OSC 777), the
// working directory (OSC 7). It only looks; the bytes go on unchanged.
package stream

import (
	"encoding/base64"
	"strings"
)

// maxOSC is the longest OSC sequence the scanner keeps; longer ones (an
// image, a clipboard write) are skipped.
const maxOSC = 8 << 10

// Scanner finds OSC sequences in a byte stream that arrives in pieces.
type Scanner struct {
	// On is called with each OSC sequence's code ("7501") and the text
	// after its first ';'. It runs on the reading goroutine.
	On func(code, data string)

	state int
	buf   []byte
	skip  bool // the sequence outgrew maxOSC
}

const (
	ground = iota
	escape // after ESC
	osc    // inside ESC ] …
	oscEsc // after ESC inside an OSC: ESC \ ends it
)

// Feed scans p.
func (s *Scanner) Feed(p []byte) {
	for _, b := range p {
		switch s.state {
		case ground:
			if b == 0x1b {
				s.state = escape
			}
		case escape:
			if b == ']' {
				s.state, s.buf, s.skip = osc, s.buf[:0], false
			} else if b != 0x1b {
				s.state = ground
			}
		case osc:
			switch b {
			case 0x07: // BEL
				s.end()
			case 0x1b:
				s.state = oscEsc
			case 0x18, 0x1a: // CAN, SUB abort it
				s.state = ground
			default:
				if len(s.buf) < maxOSC {
					s.buf = append(s.buf, b)
				} else {
					s.skip = true
				}
			}
		case oscEsc:
			if b == '\\' {
				s.end()
			} else {
				// ESC then something else: the OSC was cut short and a new
				// escape sequence begins.
				s.state = escape
				if b == ']' {
					s.state, s.buf, s.skip = osc, s.buf[:0], false
				}
			}
		}
	}
}

func (s *Scanner) end() {
	s.state = ground
	if s.skip || s.On == nil {
		return
	}
	code, data, _ := strings.Cut(string(s.buf), ";")
	s.On(code, data)
}

// Status is a program's report in the Program Status Protocol (OSC 7501).
type Status struct {
	State    string // idle, working, blocked, done, error, clear
	Kind     string // with blocked: permission, question, auth
	App      string
	Progress string // with working: 0–100, when known
	Message  string
	Title    string
}

// ParseStatus parses the data of an OSC 7501 report, "state=…:key=…"; ok
// is false for a query ("?") or a report without a state.
func ParseStatus(data string) (st Status, ok bool) {
	for _, field := range strings.Split(data, ":") {
		k, v, _ := strings.Cut(field, "=")
		switch k {
		case "state":
			st.State = v
		case "kind":
			st.Kind = v
		case "app":
			st.App = v
		case "progress":
			st.Progress = v
		case "msg":
			st.Message = decode(v)
		case "title":
			st.Title = decode(v)
		}
	}
	return st, st.State != ""
}

// decode reads a base64 field, standard or URL-safe, padded or not; text
// that is not base64 comes back as it is.
func decode(v string) string {
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(v); err == nil {
			return string(b)
		}
	}
	return v
}
