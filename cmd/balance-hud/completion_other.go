//go:build !linux && !windows

package main

import (
	"context"
	"errors"
)

func notifyCompletion(context.Context, int) error {
	return errors.New("desktop notifications unavailable")
}
