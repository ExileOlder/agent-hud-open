//go:build !windows && (!linux || !cgo)

package main

import "errors"

func runDesktop(func() []result) error {
	return errors.New("native desktop is not available in this build")
}
