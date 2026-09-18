//go:build !unix

package dolt

import "os/exec"

func configureEngineCmd(*exec.Cmd) {}

func killEngineCmd(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
