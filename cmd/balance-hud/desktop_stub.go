//go:build !windows && (!linux || !cgo)

package main

import "errors"

func runDesktop(func() <-chan result, ...settingsTools) error {
	return errors.New("native desktop is not available in this build")
}
