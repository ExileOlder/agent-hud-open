//go:build !windows

package quota

import "os/exec"

func configureProcess(*exec.Cmd) {}
