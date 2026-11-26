package main

import (
	"context"
	"errors"
	"strings"
	"syscall"
	"unsafe"
)

type credential struct {
	Flags, Kind             uint32
	Target, Comment         *uint16
	LastWritten             syscall.Filetime
	BlobSize                uint32
	Blob                    *uint16
	Persist, AttributeCount uint32
	Attributes              uintptr
	TargetAlias, UserName   *uint16
}

var credRead = syscall.NewLazyDLL("advapi32.dll").NewProc("CredReadW")
var credFree = syscall.NewLazyDLL("advapi32.dll").NewProc("CredFree")

func savedVibeKey(_ context.Context) (string, error) {
	target := utf16("BalanceHUD:MistralVibe")
	var c *credential
	ok, _, _ := credRead.Call(uintptr(unsafe.Pointer(target)), 1, 0, uintptr(unsafe.Pointer(&c)))
	if ok == 0 || c == nil {
		return "", errors.New("add a BalanceHUD:MistralVibe generic credential in Windows Credential Manager")
	}
	defer credFree.Call(uintptr(unsafe.Pointer(c)))
	if c.Blob == nil || c.BlobSize == 0 || c.BlobSize%2 != 0 {
		return "", errors.New("invalid Mistral Vibe credential blob")
	}
	return strings.TrimSpace(syscall.UTF16ToString(unsafe.Slice(c.Blob, int(c.BlobSize)/2))), nil
}
func setupVibeKey(_ context.Context) error {
	return errors.New("use Credential Manager → Windows Credentials → Add a generic credential")
}
