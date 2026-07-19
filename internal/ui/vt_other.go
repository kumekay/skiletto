//go:build !windows

package ui

import "os"

// EnableVT reports whether the terminal attached to f renders VT escape
// sequences. Unix terminals interpret them natively, so this is always
// true; the Windows build must enable console VT processing first.
func EnableVT(_ *os.File) bool {
	return true
}
