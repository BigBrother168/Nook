package stream

import (
	"slices"
	"testing"
)

func TestURLFinder(t *testing.T) {
	var got []string
	f := &URLFinder{On: func(u string) { got = append(got, u) }}
	// Vite, in color, the address split between two reads.
	f.Feed([]byte("  \x1b[32m➜\x1b[39m  \x1b[1mLocal\x1b[22m:   \x1b[36mhttp://localhost:51"))
	f.Feed([]byte("73/\x1b[39m\r\n"))
	f.Feed([]byte("Listening on http://0.0.0.0:8000.\r\n"))
	f.Feed([]byte("again http://localhost:5173 and https://example.com/x\r\n"))
	f.Feed([]byte("api at http://127.0.0.1:3000/api/v1?x=1)\n"))
	f.Feed([]byte("Serving HTTP on :: port 8080 (http://[::]:8080/) ...\n"))
	f.Feed([]byte("or http://[::1] alone\n"))
	want := []string{"http://localhost:5173", "http://localhost:8000", "http://127.0.0.1:3000/api/v1?x=1", "http://localhost:8080", "http://[::1]"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}
