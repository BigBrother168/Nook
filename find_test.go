package main

import "testing"

func TestFindCount(t *testing.T) {
	cases := []struct {
		text            string
		total, selected int
		searching       bool
		want            string
	}{
		{"", 0, 0, false, ""},
		{"err", 0, 0, true, "找中…"},
		{"err", 0, 0, false, "没找到"},
		{"err", 5, 0, true, "5"},
		{"err", 5, 2, false, "2/5"},
	}
	for _, c := range cases {
		if got := findCount(c.text, c.total, c.selected, c.searching); got != c.want {
			t.Errorf("findCount(%q, %d, %d, %v) = %q, want %q", c.text, c.total, c.selected, c.searching, got, c.want)
		}
	}
}

// The font size counts from the terminal's default, 13, until one is set.
func TestFontSizeFromDefault(t *testing.T) {
	a := testApp()
	if a.fontSize() != defaultFontSize {
		t.Fatalf("unset: %d", a.fontSize())
	}
	a.setFontSize(a.fontSize() + 1)
	if a.st.FontSize != defaultFontSize+1 {
		t.Fatalf("one larger than the default: %d", a.st.FontSize)
	}
}

// Choosing a font keeps the size as it was: the default stays the default.
func TestFontKeepsSize(t *testing.T) {
	a := testApp()
	a.st.TermFont = "Cascadia Code"
	a.applyFont()
	if a.st.FontSize != 0 || a.termFont().Size != 0 {
		t.Fatalf("size %d, font %+v", a.st.FontSize, a.termFont())
	}
}
