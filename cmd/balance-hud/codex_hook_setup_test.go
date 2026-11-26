package main

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	unicode16 "unicode/utf16"
)

func TestCodexApprovalSetupMergesAndKeepsExistingTrustInputs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.json")
	const prior = `{"scope":"untouched","hooks":{"Stop":[{"hooks":[{"type":"command","command":"keep stop"}]}],"PermissionRequest":[{"matcher":"Shell","hooks":[{"type":"command","command":"custom","timeout":21}]}]}}`
	if err := os.WriteFile(path, []byte(prior), 0600); err != nil {
		t.Fatal(err)
	}
	const exe = "/some user's/AI/Balance HUD/balance-hud"
	if err := addCodexApprovalHook(path, exe, false); err != nil {
		t.Fatal(err)
	}
	buf, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Scope string
		Hooks map[string][]struct {
			Matcher string
			Hooks   []codexApprovalHook
		}
	}
	if err := json.Unmarshal(buf, &parsed); err != nil {
		t.Fatal(err)
	}
	g := parsed.Hooks["PermissionRequest"]
	if parsed.Scope != "untouched" || parsed.Hooks["Stop"][0].Hooks[0].Command != "keep stop" ||
		len(g) != 2 || g[0].Matcher != "Shell" || g[0].Hooks[0].Timeout != 21 ||
		g[1].Hooks[0].Command != `'/some user'\''s/AI/Balance HUD/balance-hud' -completed codex -hook` {
		t.Fatalf("configuration changed unexpectedly: %s", buf)
	}
	if err := addCodexApprovalHook(path, exe, false); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, buf) {
		t.Fatal("repeated setup altered the existing hook or its review inputs")
	}
}

func TestCodexApprovalSetupNeverReplacesInvalidConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.json")
	for _, malformed := range []string{`{"hooks":`, `null`, `{"hooks":[]}`, `{"hooks":{"PermissionRequest":{}}}`} {
		if err := os.WriteFile(path, []byte(malformed), 0600); err != nil {
			t.Fatal(err)
		}
		if err := addCodexApprovalHook(path, "/balance-hud", false); err == nil {
			t.Fatalf("malformed config accepted: %s", malformed)
		}
		if got, err := os.ReadFile(path); err != nil || string(got) != malformed {
			t.Fatal("invalid file was replaced")
		}
	}
}

func TestCodexWindowsCallbackUsesLiteralExecutablePath(t *testing.T) {
	const exe = `C:\Users\O'Brien\工具\BalanceHUD.exe`
	h := codexApprovalHandler(exe, true)
	const prefix = "powershell.exe -NoLogo -NoProfile -NonInteractive -EncodedCommand "
	w, ok := strings.CutPrefix(h.CommandWindows, prefix)
	data, err := base64.StdEncoding.DecodeString(w)
	if !ok || err != nil || len(data)%2 != 0 {
		t.Fatalf("cannot decode Windows invocation: %s", h.CommandWindows)
	}
	buf := make([]uint16, len(data)/2)
	for i := range buf {
		buf[i] = binary.LittleEndian.Uint16(data[2*i:])
	}
	if want := `& 'C:\Users\O''Brien\工具\BalanceHUD.exe' -completed codex -hook`; string(unicode16.Decode(buf)) != want {
		t.Fatalf("wrong Windows callback: %q", unicode16.Decode(buf))
	}
}
