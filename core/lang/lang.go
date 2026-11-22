// Package lang picks Nook's texts in the language chosen: each text is
// written in Chinese and in English, side by side where it is used.
package lang

import "strings"

// English is whether texts show in English rather than Chinese; Set sets
// it as Nook starts, before any text shows.
var English bool

// L returns zh, or en in English.
func L(zh, en string) string {
	if English {
		return en
	}
	return zh
}

// Set sets the language: "zh", "en", or "" for the system's, given its
// preferred languages, the first first.
func Set(setting string, preferred []string) {
	switch setting {
	case "zh":
		English = false
	case "en":
		English = true
	default:
		English = len(preferred) > 0 && !strings.HasPrefix(strings.ToLower(preferred[0]), "zh")
	}
}

// Text is a text in both languages, chosen when it shows: for what is
// made before the language is set, as a package's errors.
type Text struct{ Zh, En string }

func (t Text) String() string { return L(t.Zh, t.En) }

// Error is an error whose message is a Text.
type Error Text

func (e Error) Error() string { return L(e.Zh, e.En) }
