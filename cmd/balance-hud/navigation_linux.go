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
