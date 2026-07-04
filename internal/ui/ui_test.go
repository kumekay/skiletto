package ui

import (
	"strings"
	"testing"
)

func TestSelectInteractiveMatrix(t *testing.T) {
	cases := []struct {
		name string
		opts SelectOpts
		want bool // want interactive
	}{
		{"all conditions met", SelectOpts{StdinTTY: true, StdoutTTY: true}, true},
		{"stdin not a tty", SelectOpts{StdinTTY: false, StdoutTTY: true}, false},
		{"stdout not a tty", SelectOpts{StdinTTY: true, StdoutTTY: false}, false},
		{"no-input set", SelectOpts{StdinTTY: true, StdoutTTY: true, NoInput: true}, false},
		{"CI set to 1", SelectOpts{StdinTTY: true, StdoutTTY: true, CI: "1"}, false},
		{"CI set to any non-empty value", SelectOpts{StdinTTY: true, StdoutTTY: true, CI: "false"}, false},
		{"nothing available", SelectOpts{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := Select(c.opts)
			_, nonInteractive := p.(ErrPrompter)
			gotInteractive := !nonInteractive
			if gotInteractive != c.want {
				t.Errorf("Select(%+v) interactive = %v, want %v", c.opts, gotInteractive, c.want)
			}
		})
	}
}

func TestErrPrompterReturnsActionableError(t *testing.T) {
	opts := []Option{
		{Label: "skills/pdf", Value: "skills/pdf", Hint: "skiletto add r//skills/pdf"},
		{Label: "skills/web", Value: "skills/web", Hint: "skiletto add r//skills/web"},
	}
	sel, err := ErrPrompter{}.MultiSelect("Select skills", opts)
	if err == nil {
		t.Fatal("want an error from the non-interactive prompter")
	}
	if sel != nil {
		t.Errorf("selection = %v, want nil", sel)
	}
	msg := err.Error()
	for _, want := range []string{"skiletto add r//skills/pdf", "skiletto add r//skills/web", "--all", "//path"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error missing %q:\n%s", want, msg)
		}
	}
}
