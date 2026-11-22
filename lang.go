package main

import (
	"fmt"

	"nook/core/lang"
)

// L returns zh, or en when Nook speaks English (core/lang).
func L(zh, en string) string { return lang.L(zh, en) }

// count says how many of a thing: "3 条连接" in Chinese, "3 connections"
// or "1 connection" in English.
func count(n int, zh, one, many string) string {
	if lang.English {
		if n == 1 {
			return "1 " + one
		}
		return fmt.Sprintf("%d %s", n, many)
	}
	return fmt.Sprintf("%d %s", n, zh)
}

// conns is count for connections.
func conns(n int) string { return count(n, "条连接", "connection", "connections") }
