package cli

import "testing"

func TestProgressEnabledMatrix(t *testing.T) {
	cases := []struct {
		name      string
		stderrTTY bool
		noInput   bool
		ci        string
		want      bool
	}{
		{"tty", true, false, "", true},
		{"not a tty", false, false, "", false},
		{"no-input", true, true, "", false},
		{"CI set", true, false, "1", false},
		{"CI set to any non-empty value", true, false, "false", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := progressEnabled(c.stderrTTY, c.noInput, c.ci); got != c.want {
				t.Errorf("progressEnabled(%v, %v, %q) = %v, want %v",
					c.stderrTTY, c.noInput, c.ci, got, c.want)
			}
		})
	}
}
