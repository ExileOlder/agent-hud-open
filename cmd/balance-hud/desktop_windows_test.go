package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/ExileOlder/agent-hud-open/internal/codexnotice"
	"github.com/ExileOlder/agent-hud-open/internal/quota"
)

func TestWindowsNativeSmoke(t *testing.T) {
	if os.Getenv("BALANCE_HUD_GUI_CHILD") == "1" {
		// Drive the exact desktop loop, with quota/account data independent of
		// credentials. A question arrives on demand via stdin.
		events := make(chan codexnotice.Event, 1)
		go func() {
			b := make([]byte, 1)
			if _, err := os.Stdin.Read(b); err == nil {
				events <- codexnotice.Event{Thread: "12345678-1234-1234-1234-123456789012", ID: "question:fixture", Question: true}
			}
		}()
		fetch := func() []result {
			return []result{
				{Provider: "codex", Accounts: []quota.Account{{Label: "Codex", Windows: []quota.Window{{Name: "7d", Remaining: 82}}}}},
				{Provider: "antigravity", Accounts: []quota.Account{
					{Label: "one@example.test", Windows: []quota.Window{{Name: "7d", Remaining: 64}}},
					{Label: "two@example.test", Windows: []quota.Window{{Name: "7d", Remaining: 45}}},
				}},
				{Provider: "mistral", Accounts: []quota.Account{{Label: "Mistral Vibe", Balance: "137.14 / 150.00 USD", Windows: []quota.Window{{Name: "Subscription", Remaining: 91.426}}}}},
				{Provider: "claude", Accounts: []quota.Account{{Label: "Claude Code", Windows: []quota.Window{{Name: "5h", Remaining: 63}}}}},
			}
		}
		if err := runDesktop(fetch, settingsTools{notices: events}); err != nil {
			t.Fatal(err)
		}
		return
	}
	if os.Getenv("BALANCE_HUD_GUI_SMOKE") != "1" {
		t.Skip("native window smoke check runs on the Windows CI runner")
	}
	base := t.TempDir()
	t.Setenv("APPDATA", base)
	if err := (placement{Edge: 3, Offset: .4, Numbers: false}).save(); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	app := exec.CommandContext(ctx, exe, "-test.run=^TestWindowsNativeSmoke$")
	app.Env = append(os.Environ(), "BALANCE_HUD_GUI_CHILD=1")
	input, err := app.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	app.Stdout, app.Stderr = &output, &output
	if err = app.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if app.ProcessState == nil {
			app.Process.Kill()
			app.Wait()
		}
		if t.Failed() {
			t.Log(output.String())
		}
	}()

	wait := func(description string, check func() bool) {
		t.Helper()
		deadline := time.Now().Add(4 * time.Second)
		for time.Now().Before(deadline) {
			if check() {
				return
			}
			time.Sleep(12 * time.Millisecond)
		}
		t.Fatalf("Windows: %s", description)
	}
	find := func(title string) uintptr {
		h := u("FindWindowW", uintptr(unsafe.Pointer(utf16("BalanceHUD"))), uintptr(unsafe.Pointer(utf16(title))))
		var owner uint32
		if h != 0 {
			u("GetWindowThreadProcessId", h, uintptr(unsafe.Pointer(&owner)))
		}
		if owner != uint32(app.Process.Pid) {
			return 0
		}
		return h
	}
	visible := func(w uintptr) bool { return w != 0 && u("IsWindowVisible", w) != 0 }
	client := func(w uintptr) rect {
		var r rect
		u("GetClientRect", w, uintptr(unsafe.Pointer(&r)))
		return r
	}
	post := func(w uintptr, msg, arg uintptr, x, y int32) {
		t.Helper()
		if u("PostMessageW", w, msg, arg, uintptr(uint16(x))|uintptr(uint16(y))<<16) == 0 {
			t.Fatalf("PostMessageW(%x) failed", msg)
		}
	}
	var bar, tip, prefs, notice uintptr
	wait("four-position float is not visible", func() bool {
		bar, tip = find("Balance HUD"), find("Balance HUD quota details")
		prefs, notice = find("Balance HUD Settings"), find("Balance HUD completion")
		return visible(bar) && tip != 0 && prefs != 0 && notice != 0
	})
	if r := client(bar); r.Right != barItem || r.Bottom != 4*barItem+2*barPadding {
		t.Fatalf("initial float geometry: %#v", r)
	}
	// Move the real test-runner cursor as well as posting the message. With
	// only a synthetic WM_MOUSEMOVE, TrackMouseEvent correctly sees the
	// cursor outside and immediately schedules a fade-out.
	var before point
	if u("GetCursorPos", uintptr(unsafe.Pointer(&before))) != 0 {
		defer u("SetCursorPos", uintptr(before.X), uintptr(before.Y))
	}
	pointOnMark := point{46, 50}
	if u("ClientToScreen", bar, uintptr(unsafe.Pointer(&pointOnMark))) == 0 ||
		u("SetCursorPos", uintptr(pointOnMark.X), uintptr(pointOnMark.Y)) == 0 {
		t.Fatal("cannot place the native runner cursor on the mark")
	}
	post(bar, 0x200, 0, 46, 50)
	sawIntermediate := false
	maxAlpha := byte(0)
	defer func() {
		if t.Failed() {
			t.Logf("quota card visibility=%t maximum opacity=%d", visible(tip), maxAlpha)
		}
	}()
	wait("quota card did not reach full opacity", func() bool {
		var alpha byte
		if visible(tip) && u("GetLayeredWindowAttributes", tip, 0, uintptr(unsafe.Pointer(&alpha)), 0) != 0 {
			maxAlpha = max(maxAlpha, alpha)
			if alpha > 0 && alpha < 255 {
				sawIntermediate = true
			}
			return alpha == 255
		}
		return false
	})
	if !sawIntermediate {
		t.Fatal("quota card skipped intermediate opacity")
	}
	size := client(tip)
	post(tip, 0x202, 0, size.Right-20, size.Bottom/2)
	wait("gear did not open standalone preferences", func() bool { return visible(prefs) && !visible(tip) })
	// Accounts → Claude Code · show in HUD.
	post(prefs, 0x202, 0, 50, 128)
	post(prefs, 0x202, 0, 510, 107+5*49+20)
	wait("optional fifth mark did not resize the float", func() bool {
		return client(bar).Bottom == 5*barItem+2*barPadding
	})
	var prefsOn placement
	data, err := os.ReadFile(settingsPath())
	if err != nil || json.Unmarshal(data, &prefsOn) != nil || !prefsOn.ShowClaude {
		t.Fatalf("preference not persisted: %s %v", strings.TrimSpace(string(data)), err)
	}
	post(prefs, 0x10, 0, 0, 0)
	wait("preferences did not close independently", func() bool { return !visible(prefs) && visible(bar) })
	if _, err := input.Write([]byte("q")); err != nil {
		t.Fatal(err)
	}
	wait("question notice did not appear", func() bool { return visible(notice) })
	post(notice, 0x205, 0, 40, 30)
	wait("question notice did not dismiss", func() bool { return !visible(notice) })
	if err := notifyCompletion(ctx, 5); err != nil {
		t.Fatal(err)
	}
	wait("separate-process completion did not show a notice", func() bool { return visible(notice) })
	post(bar, 0x10, 0, 0, 0)
	if err := app.Wait(); err != nil {
		t.Fatal(err)
	}
}
