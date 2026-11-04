//go:build !darwin && !linux

package modelruntime

import "os/exec"

// Other platforms terminate the owned child only. In particular, Windows needs
// Job Objects for a future equivalent of the Unix process-group cleanup.
func prepareProcess(cmd *exec.Cmd)    {}
func stopProcess(cmd *exec.Cmd) error { return cmd.Process.Kill() }
func killProcess(cmd *exec.Cmd) error { return cmd.Process.Kill() }
func cleanupProcess(cmd *exec.Cmd)    {}
