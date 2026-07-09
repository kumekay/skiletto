//go:build windows

package engine

import (
	"os/exec"
	"syscall"
)

// hookCmd builds the process for a hook command with dir appended as its
// last argument, run through cmd.exe. CmdLine is set directly because Go's
// default argv quoting does not match cmd.exe's parsing rules.
func hookCmd(command, dir string) *exec.Cmd {
	c := exec.Command("cmd")
	c.SysProcAttr = &syscall.SysProcAttr{
		CmdLine: `/S /C "` + command + ` "` + dir + `""`,
	}
	return c
}
