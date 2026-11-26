package main

import (
	"context"
	"errors"
	"os/exec"
	"strings"
)

func savedVibeKey(ctx context.Context) (string, error) {
	out, e := exec.CommandContext(ctx, "secret-tool", "lookup", "application", "BalanceHUD", "provider", "mistral-vibe").Output()
	if e != nil {
		return "", errors.New("Mistral Vibe key not available in Secret Service")
	}
	return strings.TrimSpace(string(out)), nil
}
func setupVibeKey(ctx context.Context) error {
	if _, e := exec.LookPath("secret-tool"); e != nil {
		return errors.New("install libsecret-tools first")
	}
	secret, e := exec.CommandContext(ctx, "zenity", "--password", "--title=Balance HUD — Mistral Vibe", "--text=Enter your Mistral Vibe subscription key").Output()
	if e != nil {
		return errors.New("Mistral Vibe setup canceled or the password dialog could not open")
	}
	defer clear(secret)
	s := strings.TrimSpace(string(secret))
	if s == "" {
		return errors.New("empty Mistral Vibe key")
	}
	cmd := exec.CommandContext(ctx, "secret-tool", "store", "--label=Balance HUD — Mistral Vibe", "application", "BalanceHUD", "provider", "mistral-vibe")
	cmd.Stdin = strings.NewReader(s)
	if cmd.Run() != nil {
		return errors.New("Secret Service could not save the Mistral Vibe key")
	}
	return nil
}
