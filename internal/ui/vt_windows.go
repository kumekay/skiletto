//go:build windows

package ui

import (
	"os"

	"golang.org/x/sys/windows"
)

// EnableVT turns on virtual-terminal processing for the console attached
// to f, so the \r and erase-line sequences Progress emits render as
// in-place updates instead of literal escape fragments. It reports whether
// the escapes are safe: a legacy conhost that rejects the mode gets no
// progress rather than garbage.
func EnableVT(f *os.File) bool {
	h := windows.Handle(f.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(h, &mode); err != nil {
		return false
	}
	if mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0 {
		return true
	}
	return windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil
}
