//go:build windows

package workiq

import "os/exec"

// setPgid is a no-op on Windows; process-group semantics differ and the default
// behavior is sufficient for our use.
func setPgid(cmd *exec.Cmd) {}

// terminate kills the child process. Windows does not have Unix process groups,
// so grandchildren are handled by the OS job/console cleanup.
func terminate(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
