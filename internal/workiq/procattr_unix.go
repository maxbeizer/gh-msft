//go:build !windows

package workiq

import (
	"os/exec"
	"syscall"
)

// setPgid places the child in its own process group so the whole tree
// (npx -> node -> native workiq) can be signaled together.
func setPgid(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// terminate kills the child's entire process group, falling back to the single
// process if the group id cannot be resolved. This prevents orphaned WorkIQ
// processes (and their popup windows) from lingering after we're done.
func terminate(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	if pgid, err := syscall.Getpgid(cmd.Process.Pid); err == nil {
		_ = syscall.Kill(-pgid, syscall.SIGTERM)
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		return
	}
	_ = cmd.Process.Kill()
}
