//go:build !linux && !windows

package main

import "errors"

func openClientURI(string) error { return errors.New("client opening unavailable") }

func launchDeepSeek() error { return openClientURI("dsh://open") }
