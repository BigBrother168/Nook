package main

import (
	"slices"
	"strings"
	"testing"

	"nook/core/config"
)

func TestParseAddress(t *testing.T) {
	cases := []struct {
		in, want string
		local    bool
	}{
		{"localhost:3000", "http://localhost:3000", true},
		{"127.0.0.1:8080/x", "http://127.0.0.1:8080/x", true},
		{"http://0.0.0.0:5173/", "http://localhost:5173", true},
		{"example.com", "https://example.com", false},
		{"http://dev.localhost:3000", "http://dev.localhost:3000", true},
	}
	for _, c := range cases {
		u, ok := parseAddress(c.in)
		if !ok || u.String() != c.want || local(u) != c.local {
			t.Errorf("%q: %v %v local=%v", c.in, u, ok, u != nil && local(u))
		}
	}
	for _, bad := range []string{"", "javascript:alert(1)", "file:///C:/x", "ftp://x"} {
		if _, ok := parseAddress(bad); ok {
			t.Errorf("%q parsed", bad)
		}
	}
}

func TestPreviewsByProject(t *testing.T) {
	a := testApp()
	a.st.Projects = []config.Project{{Name: "p", Path: `E:\p`}}
	for i := 0; i < 10; i++ {
		a.addPreview(`E:\p`, "http://localhost:"+itoa(3000+i))
	}
	a.addPreview(`E:\p`, "http://localhost:3005")
	got := a.previews(`E:\p`)
	if len(got) != maxPreviews || got[0] != "http://localhost:3005" || slices.Index(got, "http://localhost:3005") != 0 || strings.Count(strings.Join(got, " "), "3005") != 1 {
		t.Fatalf("previews = %q", got)
	}
	a.addPreview("", "http://localhost:9999")
	if got := a.previews(""); len(got) != 1 {
		t.Fatalf("loose = %q", got)
	}
}

func TestLockBlocksInternet(t *testing.T) {
	a := testApp()
	a.st.Locked = true
	a.openWeb("example.com")
	if !strings.Contains(a.notice, "联网锁") || a.viewing != "" {
		t.Fatalf("notice %q viewing %q", a.notice, a.viewing)
	}
}
