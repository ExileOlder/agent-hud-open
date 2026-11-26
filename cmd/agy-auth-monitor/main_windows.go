// agy-auth-monitor preserves the console entry point for the portable Windows HUD.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	self, err := os.Executable()
	if err == nil {
		args := append([]string{"--antigravity-helper"}, os.Args[1:]...)
		cmd := exec.Command(filepath.Join(filepath.Dir(self), "BalanceHUD.exe"), args...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		err = cmd.Run()
		if err == nil {
			return
		}
		if failed, ok := err.(*exec.ExitError); ok {
			os.Exit(failed.ExitCode())
		}
	}
	fmt.Fprintln(os.Stderr, "agy-auth-monitor:", err)
	os.Exit(1)
}
