package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/ExileOlder/agent-hud-open/internal/codexnotice"
	"github.com/ExileOlder/agent-hud-open/internal/quota"
)

func TestCodexHookMessageKeepsBothUUIDHalves(t *testing.T) {
	const first, second = 0x1198feedcafedead, 0xf0e1d2c3b4a59687
	id := codexThreadFromMessage(uintptr(first), uintptr(second))
	if got := id.route(); got != "addefeca-edfe-9811-8796-a5b4c3d2e1f0" {
		t.Fatalf("received the wrong Codex UUID: %s", got)
	}
}

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
		fetch := func() <-chan result {
			updates := make(chan result, 4)
			for _, reading := range []result{
				{Provider: "codex", Accounts: []quota.Account{{Label: "Codex", Windows: []quota.Window{{Name: "7d", Remaining: 82}}}}},
				{Provider: "antigravity", Accounts: []quota.Account{
					{Label: "one@example.test", Windows: []quota.Window{{Name: "7d", Remaining: 64}}},
					{Label: "two@example.test", Windows: []quota.Window{{Name: "7d", Remaining: 45}}},
				}},
				{Provider: "mistral", Accounts: []quota.Account{{Label: "Mistral Vibe", Balance: "137.14 / 150.00 USD", Windows: []quota.Window{{Name: "Subscription", Remaining: 91.426}}}}},
				{Provider: "claude", Accounts: []quota.Account{{Label: "Claude Code", Windows: []quota.Window{{Name: "5h", Remaining: 63}}}}},
			} {
				updates <- reading
			}
			close(updates)
			return updates
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
	hookHome := t.TempDir()
	t.Setenv("CODEX_HOME", hookHome)
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
	var bar, tip, prefs, badge, inbox, paper uintptr
	wait("four-position float is not visible", func() bool {
		bar, tip = find("Balance HUD"), find("Balance HUD quota details")
		prefs, badge = find("Balance HUD Settings"), find("Balance HUD messages")
		inbox, paper = find("Balance HUD inbox"), find("Balance HUD paper flight")
		return visible(bar) && tip != 0 && prefs != 0 && badge != 0 && inbox != 0 && paper != 0
	})
	reopenCtx, stopReopen := context.WithTimeout(ctx, 4*time.Second)
	defer stopReopen()
	reopen := exec.CommandContext(reopenCtx, exe, "-test.run=^TestWindowsNativeSmoke$")
	reopen.Env = append(os.Environ(), "BALANCE_HUD_GUI_CHILD=1")
	if output, err := reopen.CombinedOutput(); err != nil || !visible(bar) {
		t.Fatalf("relaunch should leave the first native dock running: %v, %s", err, output)
	}
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
	// The original card stays mounted while changing y and width between
	// two different slots. No second opacity transition should run.
	var first, later rect
	u("GetWindowRect", tip, uintptr(unsafe.Pointer(&first)))
	onSecond := point{46, 50 + barItem}
	if u("ClientToScreen", bar, uintptr(unsafe.Pointer(&onSecond))) == 0 ||
		u("SetCursorPos", uintptr(onSecond.X), uintptr(onSecond.Y)) == 0 {
		t.Fatal("cannot move to second mark")
	}
	post(bar, 0x200, 0, 46, 50+barItem)
	crossed := false
	wait("quota card did not reach the second mark", func() bool {
		var alpha byte
		u("GetWindowRect", tip, uintptr(unsafe.Pointer(&later)))
		u("GetLayeredWindowAttributes", tip, 0, uintptr(unsafe.Pointer(&alpha)), 0)
		distance := later.Top - first.Top
		if distance > 0 && distance < barItem && alpha == 255 {
			crossed = true
		}
		return distance == barItem && alpha == 255
	})
	if !crossed {
		t.Fatal("card jumped instead of gliding between marks")
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
	hookPath := filepath.Join(hookHome, "hooks.json")
	if _, err := os.Stat(hookPath); !os.IsNotExist(err) {
		t.Fatalf("opening settings wrote hook configuration: %v", err)
	}
	post(prefs, 0x202, 0, 510, 107+7*49+20)
	wait("approval setup did not register the native Windows callback", func() bool {
		b, err := os.ReadFile(hookPath)
		return err == nil && bytes.Contains(b, []byte(`"PermissionRequest"`)) && bytes.Contains(b, []byte(`"commandWindows"`))
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
	wait("question did not launch a paper flight", func() bool { return visible(paper) })
	wait("flight did not land at dock inbox", func() bool { return visible(badge) && !visible(paper) })
	post(badge, 0x202, 0, 40, 40)
	wait("dock inbox did not open independently", func() bool { return visible(inbox) })
	post(inbox, 0x202, 0, client(inbox).Right-40, mailHeader+29)
	wait("single-row dismiss did not close the empty badge and inbox", func() bool { return !visible(badge) && !visible(inbox) })
	if err := notifyCodexHook(ctx, parseCodexHookThread("cdefabcd-1234-5678-9abc-def012345678"), true); err != nil {
		t.Fatal(err)
	}
	wait("approval did not launch a paper flight", func() bool { return visible(paper) })
	wait("approval did not land at dock inbox", func() bool { return visible(badge) && !visible(paper) })
	post(badge, 0x202, 0, 40, 40)
	wait("approval inbox did not open", func() bool { return visible(inbox) })
	post(inbox, 0x202, 0, client(inbox).Right-40, mailHeader+29)
	wait("approval could not be dismissed independently", func() bool { return !visible(badge) && !visible(inbox) })
	const finishedThread = "78654321-1234-5678-abcd-123456789012"
	stop := strings.NewReader(`{"hook_event_name":"Stop","session_id":"` + finishedThread + `","last_assistant_message":"private"}`)
	if err := runCompletion("codex", true, stop, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	wait("separate-process completion did not land", func() bool { return visible(badge) })
	var saved []savedNotice
	data, err = os.ReadFile(filepath.Join(base, "BalanceHUD", "inbox.json"))
	if err != nil || json.Unmarshal(data, &saved) != nil ||
		len(saved) != 1 || saved[0] != (savedNotice{Agent: 1, Thread: finishedThread}) ||
		bytes.Contains(data, []byte("private")) {
		t.Fatalf("routed native completion was not preserved exactly once: %s %v", data, err)
	}
	var desktop rect
	if u("SystemParametersInfoW", 48, 0, uintptr(unsafe.Pointer(&desktop)), 0) == 0 {
		t.Fatal("cannot read the native runner's work area")
	}
	bounds := func(hwnd uintptr) rect {
		var r rect
		if u("GetWindowRect", hwnd, uintptr(unsafe.Pointer(&r))) == 0 {
			t.Fatal("cannot read the native dock rectangle")
		}
		return r
	}
	changeCursor := func(q point) {
		if u("SetCursorPos", uintptr(q.X), uintptr(q.Y)) == 0 {
			t.Fatal("cannot move the native cursor for drag")
		}
	}
	origin := bounds(bar)
	grab := point{origin.Left + 46, origin.Top + 50}
	changeCursor(grab)
	down, up := uintptr(0x0002), uintptr(0x0004) // physical primary down/up
	if u("GetSystemMetrics", 23) != 0 {
		down, up = 0x0008, 0x0010
	}
	u("mouse_event", down, 0, 0, 0, 0)
	pressed := true
	defer func() {
		if pressed {
			u("mouse_event", up, 0, 0, 0, 0)
		}
	}()
	var gui struct {
		size, flags                                   uint32
		active, focus, capture, menu, moveSize, caret uintptr
		cursor                                        rect
	}
	gui.size = uint32(unsafe.Sizeof(gui))
	thread := u("GetWindowThreadProcessId", bar, 0)
	wait("native primary button did not capture the cursor", func() bool {
		return u("GetGUIThreadInfo", thread, uintptr(unsafe.Pointer(&gui))) != 0 && gui.capture == bar
	})
	width, height := desktop.Right-desktop.Left, desktop.Bottom-desktop.Top
	centerY := desktop.Top + height/3
	assertFollows := func(q point) {
		changeCursor(q)
		wait("dock stopped following the real cursor or left the badge in the old position", func() bool {
			got := bounds(bar)
			return got.Left == origin.Left+q.X-grab.X && got.Top == origin.Top+q.Y-grab.Y && !visible(badge)
		})
	}
	assertFollows(point{desktop.Left + width/3, centerY})
	assertFollows(point{desktop.Left + 2*width/3, centerY + 17})
	// The straight middle of the left outline should alternate light dashes
	// and dark gaps; a solid border or a blank repaint would fail here.
	wait("drag outline was not painted as dashes", func() bool {
		dc := u("GetDC", bar)
		if dc == 0 {
			return false
		}
		defer u("ReleaseDC", bar, dc)
		dash, gap := false, false
		for y := uintptr(106); y < 154; y++ {
			color := uint32(g("GetPixel", dc, 2, y))
			dash = dash || color == 0x00f6f4f2
			gap = gap || color == 0x00090705
		}
		return dash && gap
	})
	changeCursor(point{desktop.Right - 8, centerY + 17})
	u("mouse_event", up, 0, 0, 0, 0)
	pressed = false
	wait("drop did not snap to the right or restore its inbox badge", func() bool {
		return bounds(bar).Right == desktop.Right-3 && visible(badge)
	})
	data, err = os.ReadFile(settingsPath())
	if err != nil || json.Unmarshal(data, &prefsOn) != nil || prefsOn.Edge != 1 {
		t.Fatalf("drag did not persist the right edge: %s %v", data, err)
	}
	post(bar, 0x10, 0, 0, 0)
	if err := app.Wait(); err != nil {
		t.Fatal(err)
	}

	inboxPath := filepath.Join(base, "BalanceHUD", "inbox.json")
	snapshot := make([]savedNotice, mailLimit)
	for i := range snapshot {
		snapshot[i] = savedNotice{Agent: 1, Thread: fmt.Sprintf("cdefabcd-1234-5678-9abc-%012x", i)}
	}
	data, err = json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inboxPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	app = exec.CommandContext(ctx, exe, "-test.run=^TestWindowsNativeSmoke$")
	app.Env = append(os.Environ(), "BALANCE_HUD_GUI_CHILD=1")
	app.Stdout, app.Stderr = &output, &output
	if err := app.Start(); err != nil {
		t.Fatal(err)
	}
	wait("full inbox did not restore", func() bool {
		bar, badge, inbox = find("Balance HUD"), find("Balance HUD messages"), find("Balance HUD inbox")
		return visible(badge) && inbox != 0
	})
	post(badge, 0x202, 0, 40, 40)
	wait("full inbox did not open", func() bool { return visible(inbox) })
	rc := client(inbox)
	rows := (rc.Bottom - mailHeader - 12) / mailRow
	y := mailHeader + (rc.Bottom-mailHeader-mailScrollBottom)*rows/mailLimit/2
	post(inbox, 0x201, 1, rc.Right-9, y)
	post(inbox, 0x200, 1, rc.Right-9, rc.Bottom-mailScrollBottom-1)
	post(inbox, 0x202, 0, rc.Right-9, rc.Bottom-mailScrollBottom-1)
	post(inbox, 0x202, 0, rc.Right-58, mailHeader+(rows-1)*mailRow+49)
	wait("scrollbar did not reach and dismiss the oldest message", func() bool {
		data, err := os.ReadFile(inboxPath)
		var got []savedNotice
		if err != nil || json.Unmarshal(data, &got) != nil || len(got) != mailLimit-1 {
			return false
		}
		for i, notice := range got {
			if notice != snapshot[i] {
				t.Fatalf("scrolling dismissed the wrong message at %d: %s", i, notice.Thread)
			}
		}
		return true
	})
	post(bar, 0x10, 0, 0, 0)
	if err := app.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestWindowsSignInOpensUsableConsole(t *testing.T) {
	const marker = "BALANCE_HUD_CONSOLE_TEST_MARKER"
	if path := os.Getenv(marker); path != "" {
		status := "console"
		for _, f := range []*os.File{os.Stdin, os.Stdout, os.Stderr} {
			var mode uint32
			if err := syscall.GetConsoleMode(syscall.Handle(f.Fd()), &mode); err != nil {
				status = err.Error()
				break
			}
		}
		if err := os.WriteFile(path, []byte(status), 0600); err != nil {
			t.Fatal(err)
		}
		return
	}
	if os.Getenv("BALANCE_HUD_GUI_SMOKE") != "1" {
		t.Skip("native console check runs on the Windows CI runner")
	}
	path := filepath.Join(t.TempDir(), "sign-in console.txt")
	t.Setenv(marker, path)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := openClientTerminal(exe, "-test.run=^TestWindowsSignInOpensUsableConsole$"); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		status, err := os.ReadFile(path)
		if err == nil && len(status) != 0 {
			if string(status) != "console" {
				t.Fatalf("sign-in terminal has unusable streams: %s", status)
			}
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("sign-in terminal did not report its standard handles")
}
