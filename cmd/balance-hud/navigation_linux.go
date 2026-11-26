package main

import "os/exec"

func openClientURI(uri string) error {
	cmd := exec.Command("xdg-open", uri)
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}

func launchDeepSeek() error {
	cmd := exec.Command("gtk-launch", "deepseek-harness-tools")
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
