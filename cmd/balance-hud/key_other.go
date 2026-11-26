//go:build !linux && !windows

package main

import (
	"context"
	"errors"
)

func savedVibeKey(context.Context) (string, error) {
	return "", errors.New("Mistral credential store unavailable")
}
func setupVibeKey(context.Context) error { return errors.New("Mistral credential store unavailable") }
