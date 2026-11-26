package main

import (
	"context"
	"errors"
	"unsafe"
)

const completionMessage = 0x8011

func notifyCompletion(_ context.Context, id int) error {
	class, title := utf16("BalanceHUD"), utf16("Balance HUD")
	hwnd := u("FindWindowW", uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(title)))
	if hwnd == 0 {
		return errors.New("start Balance HUD before sending a completion reminder")
	}
	var response uintptr
	if u("SendMessageTimeoutW", hwnd, completionMessage, uintptr(id), 0, 2, 1000, uintptr(unsafe.Pointer(&response))) == 0 {
		return errors.New("Balance HUD did not accept the completion reminder")
	}
	return nil
}
