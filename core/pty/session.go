// Package pty runs a program in a pseudo-terminal that Nook owns, so that
// Nook decides its environment, knows every process it starts, and ends
// them all when a terminal or the app closes.
//
// A Session is an io.ReadWriteCloser with Resize(cols, rows int) error, the
// shape of terminal.Options.Conn: the terminal view reads what the program
// writes and writes what is typed.
package pty

import "errors"

// Options say what a Session runs.
type Options struct {
	// Command is the program and its arguments; Command[0] is looked up in
	// PATH when it has no directory.
	Command []string
	// Dir is the working directory; the user's home when empty.
	Dir string
	// Env is the whole environment of the program, "KEY=value".
	Env []string
}

// ErrClosed is returned by writes after Close.
var ErrClosed = errors.New("pty: closed")
