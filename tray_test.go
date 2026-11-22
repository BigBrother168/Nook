package main

import (
	"bytes"
	"image/png"
	"testing"
)

func TestTrayIcons(t *testing.T) {
	for _, state := range []string{"", "working", "blocked"} {
		img, err := png.Decode(bytes.NewReader(trayIcons[state]))
		if err != nil {
			t.Fatalf("%q: %v", state, err)
		}
		if b := img.Bounds(); b.Dx() != 32 || b.Dy() != 32 {
			t.Fatalf("%q: %v", state, b)
		}
	}
	plain, _ := png.Decode(bytes.NewReader(trayIcons[""]))
	busy, _ := png.Decode(bytes.NewReader(trayIcons["working"]))
	if _, _, _, a := plain.At(16, 16).RGBA(); a == 0 {
		t.Error("the icon's middle is empty")
	}
	if plain.At(26, 26) == busy.At(26, 26) {
		t.Error("no dot at the lower right")
	}
}
