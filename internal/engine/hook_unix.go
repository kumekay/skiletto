//go:build !windows

package engine

import "os/exec"

// hookCmd builds the process for a hook command, run through the POSIX
// shell so it can use arguments, env-var references, and pipelines.
func hookCmd(command string) *exec.Cmd {
	return exec.Command("sh", "-c", command)
}
