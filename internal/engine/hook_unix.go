//go:build !windows

package engine

import (
	"os/exec"
	"strings"
)

// hookCmd builds the process for a hook command with dir appended as its
// last argument, run through the POSIX shell so the configured command can
// use arguments and environment variables.
func hookCmd(command, dir string) *exec.Cmd {
	return exec.Command("sh", "-c", command+" "+shellQuote(dir))
}

// shellQuote wraps s in single quotes, escaping embedded single quotes, so
// the path survives the shell untouched.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
