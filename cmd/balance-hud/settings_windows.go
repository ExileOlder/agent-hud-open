package main

import (
	"os/exec"
	"syscall"
)

func openClientTerminal(exe string, args ...string) error {
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x10}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}

func launchVibeSetup() string {
	cmd := exec.Command("control.exe", "/name", "Microsoft.CredentialManager")
	if err := cmd.Start(); err != nil {
		return "Could not open Windows Credential Manager."
	}
	go cmd.Wait()
	return "Add a generic credential named BalanceHUD:MistralVibe, then refresh."
}
