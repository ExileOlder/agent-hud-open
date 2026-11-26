//go:build linux && !cgo

package main

import (
	"context"
	"errors"
)

func notifyHook(context.Context, int, uint32) error {
	return errors.New("completion reminders require the X11 build of Balance HUD")
}
