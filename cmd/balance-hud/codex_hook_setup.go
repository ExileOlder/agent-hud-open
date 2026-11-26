package main

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	unicode16 "unicode/utf16"
)

func codexHome() string {
	if home := os.Getenv("CODEX_HOME"); home != "" {
		return home
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".codex")
	}
	return ""
}

func setUpCodexApprovalHook() string {
	exe, err := os.Executable()
	home := codexHome()
	if err != nil || home == "" {
		return "Could not locate Balance HUD or the Codex config directory."
	}
	if err := addCodexApprovalHook(filepath.Join(home, "hooks.json"), exe, runtime.GOOS == "windows"); err != nil {
		return "Codex hook not changed: " + err.Error()
	}
	return "In Codex, open Review hooks and trust the Balance HUD callback."
}

type codexApprovalHook struct {
	Type           string `json:"type"`
	Command        string `json:"command"`
	CommandWindows string `json:"commandWindows,omitempty"`
	Timeout        int    `json:"timeout"`
}

func codexApprovalHandler(exe string, windows bool) codexApprovalHook {
	const flags = " -completed codex -hook"
	h := codexApprovalHook{
		Type: "command", Command: "'" + strings.ReplaceAll(exe, "'", "'\\''") + "'" + flags, Timeout: 5,
	}
	if windows {
		// Codex's commandWindows is a shell command, not a PowerShell
		// expression. Use the upstream HUD's explicit UTF-16 entry point.
		script := "& '" + strings.ReplaceAll(exe, "'", "''") + "'" + flags
		u := unicode16.Encode([]rune(script))
		encoded := make([]byte, 2*len(u))
		for i, code := range u {
			binary.LittleEndian.PutUint16(encoded[2*i:], code)
		}
		h.CommandWindows = "powershell.exe -NoLogo -NoProfile -NonInteractive -EncodedCommand " +
			base64.StdEncoding.EncodeToString(encoded)
	}
	return h
}

func addCodexApprovalHook(path, exe string, windows bool) error {
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		// A linked settings file is often shared across installations.
		// Update its target instead of replacing the user's symlink.
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return err
		}
		path = resolved
	}
	before, err := os.ReadFile(path)
	missing := errors.Is(err, os.ErrNotExist)
	if err != nil && !missing {
		return err
	}
	root := map[string]json.RawMessage{}
	if !missing && (json.Unmarshal(before, &root) != nil || root == nil) {
		return errors.New("hooks.json is not a JSON object")
	}
	hooks := map[string]json.RawMessage{}
	if raw, ok := root["hooks"]; ok && (json.Unmarshal(raw, &hooks) != nil || hooks == nil) {
		return errors.New("the existing hooks section is not a JSON object")
	}
	var groups []json.RawMessage
	if raw, ok := hooks["PermissionRequest"]; ok && (json.Unmarshal(raw, &groups) != nil || groups == nil) {
		return errors.New("the existing PermissionRequest section is not a list")
	}
	target := codexApprovalHandler(exe, windows)
	for _, raw := range groups {
		var group struct{ Hooks []codexApprovalHook }
		if json.Unmarshal(raw, &group) == nil {
			for _, handler := range group.Hooks {
				if handler.Type == target.Type && handler.Command == target.Command &&
					handler.CommandWindows == target.CommandWindows {
					return nil // Keep its exact command and existing trust state.
				}
			}
		}
	}
	group, err := json.Marshal(struct {
		Hooks []codexApprovalHook `json:"hooks"`
	}{[]codexApprovalHook{target}})
	if err != nil {
		return err
	}
	hooks["PermissionRequest"], err = json.Marshal(append(groups, group))
	if err != nil {
		return err
	}
	root["hooks"], err = json.Marshal(hooks)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".balance-hud-hooks-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	writeErr := enc.Encode(root)
	closeErr := tmp.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	// Do not overwrite a concurrent edit made since reading the settings.
	current, err := os.ReadFile(path)
	if missing && !errors.Is(err, os.ErrNotExist) || !missing && (err != nil || !bytes.Equal(current, before)) {
		return errors.New("hooks.json changed while preparing the callback; try again")
	}
	return os.Rename(tmp.Name(), path)
}
