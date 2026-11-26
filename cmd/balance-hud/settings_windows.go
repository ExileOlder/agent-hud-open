package main

import (
	"os/exec"
	"strings"
	"syscall"
	"unsafe"
)

func openClientTerminal(exe string, args ...string) error {
	path, err := exec.LookPath(exe)
	if err != nil {
		return err
	}
	app, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	line := []string{syscall.EscapeArg(path)}
	for _, arg := range args {
		line = append(line, syscall.EscapeArg(arg))
	}
	command, err := syscall.UTF16PtrFromString(strings.Join(line, " "))
	if err != nil {
		return err
	}
	// Let Windows assign the new console's standard handles. Starting via
	// os/exec without streams would instead pass handles to NUL.
	startup := syscall.StartupInfo{Cb: uint32(unsafe.Sizeof(syscall.StartupInfo{}))}
	var proc syscall.ProcessInformation
	const createNewConsole = 0x10
	if err := syscall.CreateProcess(app, command, nil, nil, false, createNewConsole, nil, nil, &startup, &proc); err != nil {
		return err
	}
	_ = syscall.CloseHandle(proc.Thread)
	_ = syscall.CloseHandle(proc.Process)
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
