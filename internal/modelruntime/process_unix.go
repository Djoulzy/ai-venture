//go:build darwin || linux

package modelruntime

import (
	"os/exec"
	"syscall"
)

func prepareProcess(cmd *exec.Cmd)    { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }
func stopProcess(cmd *exec.Cmd) error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) }
func killProcess(cmd *exec.Cmd) error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
func cleanupProcess(cmd *exec.Cmd)    { _ = killProcess(cmd) }
