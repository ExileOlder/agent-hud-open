//go:build !windows && (!linux || !cgo)

package codexnotice

import "errors"

func read(string) ([]Event, error) { return nil, errors.New("Codex event integration unavailable") }
