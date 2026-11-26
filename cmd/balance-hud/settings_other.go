//go:build !linux && !windows

package main

import "errors"

func openClientTerminal(string, ...string) error { return errors.New("desktop unavailable") }
func launchVibeSetup() string                    { return "Desktop unavailable." }
