// Package ui owns interactive prompting. Command handlers never import a
// prompt library directly: they call through the Prompter interface, which
// makes the "prompts are sugar for flags" contract structural. The
// non-interactive implementation returns the actionable error a script
// needs instead of blocking on input that will never come.
package ui

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// Option is one selectable choice. Label is shown in the interactive
// picker, Value is returned when the option is selected, and Hint is the
// exact command that scripts the same choice — listed by the
// non-interactive prompter so a caller without a TTY knows how to proceed.
type Option struct {
	Label string
	Value string
	Hint  string
	// Selected pre-checks the option in the interactive picker (e.g. a
	// harness detected on this machine).
	Selected bool
}

// Prompter collects a choice from the user. Implementations are either
// interactive (a real terminal picker) or non-interactive (return an
// actionable error rather than prompt).
type Prompter interface {
	MultiSelect(title string, options []Option) ([]string, error)
}

// SelectOpts captures everything that decides whether a prompt may be
// shown. TTY status is injected so tests need no real terminal.
type SelectOpts struct {
	StdinTTY  bool
	StdoutTTY bool
	NoInput   bool
	CI        string
}

// Interactive reports whether a real prompt may be shown: both streams
// must be terminals, --no-input must be absent, and the CI env var must be
// empty. Any non-empty CI value forces the non-interactive path.
func (o SelectOpts) Interactive() bool {
	return o.StdinTTY && o.StdoutTTY && !o.NoInput && o.CI == ""
}

// Select returns the interactive prompter when the environment allows it,
// otherwise the non-interactive one.
func Select(o SelectOpts) Prompter {
	if o.Interactive() {
		return huhPrompter{}
	}
	return ErrPrompter{}
}

// ErrPrompter is the non-interactive Prompter: rather than block on input
// it returns an error listing the choices and the exact command to script
// each one.
type ErrPrompter struct{}

// MultiSelect never prompts; it returns an actionable error.
func (ErrPrompter) MultiSelect(_ string, options []Option) ([]string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "source contains %d skills; pick with //path or --skill <name>, or pass --all to install every one:", len(options))
	for _, o := range options {
		fmt.Fprintf(&b, "\n  %s", o.Hint)
	}
	return nil, errors.New(b.String())
}

// IsTerminalFile reports whether f is attached to a character device (a
// terminal). It is dependency-free and good enough to gate prompting.
func IsTerminalFile(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
