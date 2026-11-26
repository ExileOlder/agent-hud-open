//go:build linux && !cgo

package main

import (
	"context"
	"errors"
)

func notifyCompletion(context.Context, int) error {
	return errors.New("completion reminders require the X11 build of Balance HUD")
}
