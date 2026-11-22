package main

import "testing"

// The address bar opens addresses, and searches words with DuckDuckGo.
func TestWebTarget(t *testing.T) {
	cases := map[string]string{
		"localhost:3000":      "http://localhost:3000",
		"go.dev/doc":          "https://go.dev/doc",
		"https://example.com": "https://example.com",
		"golang":              "https://duckduckgo.com/?q=golang",
		"how to use go mod":   "https://duckduckgo.com/?q=how+to+use+go+mod",
		"  空格 & 中文 ":          "https://duckduckgo.com/?q=%E7%A9%BA%E6%A0%BC+%26+%E4%B8%AD%E6%96%87",
		"127.0.0.1:8765/a b":  "https://duckduckgo.com/?q=127.0.0.1%3A8765%2Fa+b",
	}
	for in, want := range cases {
		u, ok := webTarget(in)
		if !ok || u.String() != want {
			t.Errorf("webTarget(%q) = %v %v, want %s", in, u, ok, want)
		}
	}
	if _, ok := webTarget("   "); ok {
		t.Error("nothing typed: nowhere to go")
	}
}
