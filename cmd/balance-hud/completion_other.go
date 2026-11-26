//go:build !linux && !windows

package main

import (
	"context"
	"errors"
)

func notifyHook(context.Context, int, uint32) error {
	return errors.New("desktop notifications unavailable")
}
