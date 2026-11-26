package main

import (
	"context"
	"encoding/binary"
	"errors"
	"unsafe"
)

const completionMessage = 0x8011
const approvalMessage = 0x8012
const codexCompletionMessage = 0x8013

func notifyHook(_ context.Context, code int, session uint32) error {
	_, err := sendHookMessage(completionMessage, uintptr(code), uintptr(session))
	return err
}

func notifyCodexHook(ctx context.Context, thread codexHookThread, approval bool) error {
	message := uintptr(codexCompletionMessage)
	if approval {
		message = approvalMessage
	}
	accepted, err := sendHookMessage(message, uintptr(binary.LittleEndian.Uint64(thread[:8])), uintptr(binary.LittleEndian.Uint64(thread[8:])))
	if !approval && err == nil && accepted == 0 {
		// A running older build does not recognize the routed completion
		// message. It can still show the same reminder with its generic hook.
		return notifyHook(ctx, codexCompletionCode, 0)
	}
	return err
}

func codexThreadFromMessage(first, second uintptr) (id codexHookThread) {
	binary.LittleEndian.PutUint64(id[:8], uint64(first))
	binary.LittleEndian.PutUint64(id[8:], uint64(second))
	return
}

func sendHookMessage(message, wparam, lparam uintptr) (uintptr, error) {
	class, title := utf16("BalanceHUD"), utf16("Balance HUD")
	hwnd := u("FindWindowW", uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(title)))
	if hwnd == 0 {
		return 0, errors.New("start Balance HUD before sending a completion reminder")
	}
	var response uintptr
	if u("SendMessageTimeoutW", hwnd, message, wparam, lparam, 2, 1000, uintptr(unsafe.Pointer(&response))) == 0 {
		return 0, errors.New("Balance HUD did not accept the completion reminder")
	}
	return response, nil
}
