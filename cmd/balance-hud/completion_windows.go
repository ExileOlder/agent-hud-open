package main

import (
	"context"
	"encoding/binary"
	"errors"
	"unsafe"
)

const completionMessage = 0x8011
const approvalMessage = 0x8012

func notifyHook(_ context.Context, code int, session uint32) error {
	return sendHookMessage(completionMessage, uintptr(code), uintptr(session))
}

func notifyApproval(_ context.Context, thread approvalThread) error {
	return sendHookMessage(approvalMessage, uintptr(binary.LittleEndian.Uint64(thread[:8])), uintptr(binary.LittleEndian.Uint64(thread[8:])))
}

func approvalFromMessage(first, second uintptr) (id approvalThread) {
	binary.LittleEndian.PutUint64(id[:8], uint64(first))
	binary.LittleEndian.PutUint64(id[8:], uint64(second))
	return
}

func sendHookMessage(message, wparam, lparam uintptr) error {
	class, title := utf16("BalanceHUD"), utf16("Balance HUD")
	hwnd := u("FindWindowW", uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(title)))
	if hwnd == 0 {
		return errors.New("start Balance HUD before sending a completion reminder")
	}
	var response uintptr
	if u("SendMessageTimeoutW", hwnd, message, wparam, lparam, 2, 1000, uintptr(unsafe.Pointer(&response))) == 0 {
		return errors.New("Balance HUD did not accept the completion reminder")
	}
	return nil
}
