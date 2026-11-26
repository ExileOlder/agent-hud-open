package main

import (
	"os"
	"os/exec"
)

func openClientTerminal(exe string, args ...string) error {
	target, err := exec.LookPath(exe)
	if err != nil {
		return err
	}
	terminal, err := exec.LookPath("gnome-terminal")
	prefix := "--"
	if err != nil {
		terminal, err = exec.LookPath("x-terminal-emulator")
		prefix = "-e"
	}
	if err != nil {
		return err
	}
	cmd := exec.Command(terminal, append([]string{prefix, target}, args...)...)
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
func launchVibeSetup() string {
	exe, err := os.Executable()
	if err != nil {
		return "Could not locate Balance HUD."
	}
	cmd := exec.Command(exe, "-setup-mistral")
	if err := cmd.Start(); err != nil {
		return "Could not open key setup."
	}
	go cmd.Wait()
	return "Save the key in the separate dialog, then refresh balances."
}
