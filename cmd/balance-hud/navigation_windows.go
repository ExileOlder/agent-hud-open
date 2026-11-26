package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

var shell32 = syscall.NewLazyDLL("shell32.dll")

func openClientURI(uri string) error {
	r := call(shell32, "ShellExecuteW", 0, uintptr(unsafe.Pointer(utf16("open"))), uintptr(unsafe.Pointer(utf16(uri))), 0, 0, 1)
	if r <= 32 {
		return fmt.Errorf("no registered handler for this client (%d)", r)
	}
	return nil
}

func launchDeepSeek() error { return openClientURI("dsh://open") }
