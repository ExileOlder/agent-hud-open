"""Exercise the packaged native HUD on a private Xvfb display."""
import ctypes as c
import json
import os
from pathlib import Path
import re
import selectors
import subprocess as sp
import sys
import tempfile
import time
from typing import NamedTuple


class Window(NamedTuple):
    xid: int
    x: int
    y: int
    w: int
    h: int


def wait(why, action, seconds=5):
    until = time.monotonic() + seconds
    while time.monotonic() < until:
        if result := action():
            return result
        time.sleep(.015)
    raise AssertionError(why)


class Pointer:
    def __init__(self, display):
        self.x11 = c.CDLL("libX11.so.6")
        self.xtst = c.CDLL("libXtst.so.6")
        self.x11.XOpenDisplay.argtypes = [c.c_char_p]
        self.x11.XOpenDisplay.restype = c.c_void_p
        self.x11.XSync.argtypes = [c.c_void_p, c.c_int]
        self.x11.XCloseDisplay.argtypes = [c.c_void_p]
        self.xtst.XTestFakeMotionEvent.argtypes = [
            c.c_void_p, c.c_int, c.c_int, c.c_int, c.c_ulong,
        ]
        self.xtst.XTestFakeButtonEvent.argtypes = [
            c.c_void_p, c.c_uint, c.c_int, c.c_ulong,
        ]
        self.dpy = self.x11.XOpenDisplay(display.encode())
        if not self.dpy:
            raise RuntimeError(f"cannot connect test pointer to {display}")

    def move(self, x, y):
        assert self.xtst.XTestFakeMotionEvent(self.dpy, 0, x, y, 0)
        self.x11.XSync(self.dpy, 0)

    def button(self, down):
        assert self.xtst.XTestFakeButtonEvent(self.dpy, 1, down, 0)
        self.x11.XSync(self.dpy, 0)

    def click(self, x, y):
        self.move(x, y)
        self.button(True)
        self.button(False)

    def close(self):
        self.x11.XCloseDisplay(self.dpy)


def smoke(exe, home, display, log):
    env = {
        "PATH": f"{home}:/usr/bin:/bin",
        "LC_ALL": "C.UTF-8",
        "HOME": str(home),
        "XDG_CONFIG_HOME": str(home),
        "DSH_HOME": str(home),
        "CODEX_HOME": str(home),
        "DBUS_SESSION_BUS_ADDRESS": f"unix:path={home}/no-session-bus",
        "DISPLAY": display,
        "ROUTES": str(home / "routes"),
    }
    config = home / "BalanceHUD"
    config.mkdir()
    prefs = config / "appearance.json"
    prefs.write_text('{"Edge":3,"Offset":0.4,"Numbers":false}')
    store = config / "inbox.json"
    route = home / "routes"
    opener = home / "xdg-open"
    opener.write_text('#!/bin/sh\nprintf "%s\\n" "$1" >> "$ROUTES"\n')
    opener.chmod(0o700)
    fake = str(home / "missing-provider")
    args = [exe, "-codex", fake, "-antigravity", fake, "-claude", fake]
    pointer = Pointer(display)
    app = None

    def window(suffix=""):
        info = sp.run(["xwininfo", "-name", "Balance HUD" + suffix],
                      env=env, capture_output=True, text=True, check=False).stdout
        if "Map State: IsViewable" not in info:
            return None
        xid = int(re.search(r"Window id: (0x[0-9a-f]+)", info)[1], 16)
        keys = ("Absolute upper-left X", "Absolute upper-left Y", "Width", "Height")
        return Window(xid, *(int(re.search(rf"{key}:\s*(-?\d+)", info)[1]) for key in keys))

    def start():
        return sp.Popen(args, env=env, stdout=log, stderr=log)

    def stop():
        if app is not None and app.poll() is None:
            app.terminate()
            app.wait(timeout=5)

    try:
        pointer.move(700, 200)
        app = start()
        origin = wait("the float did not open", window)
        assert (origin.x, origin.w, origin.h) == (3, 92, 376), origin
        duplicate = sp.run(args, env=env, capture_output=True, timeout=4)
        assert duplicate.returncode == 0 and window() == origin, duplicate.stderr

        thread = "cdefabcd-1234-5678-9abc-def012345678"
        event = json.dumps({"hook_event_name": "PermissionRequest", "session_id": thread,
                            "tool_input": {"command": "private-fixture"}})
        cb = sp.run([exe, "-completed", "codex", "-hook"], env=env, input=event,
                    text=True, capture_output=True, check=True, timeout=4)
        assert not cb.stdout and not cb.stderr, cb
        first = wait("the paper plane never took off", lambda: window(" paper flight"))
        samples = {first}
        while air := window(" paper flight"):
            samples.add(air)
            if len(samples) >= 5:
                break
            time.sleep(.010)
        assert len(samples) >= 5, samples
        badge = wait("the paper plane did not land", lambda: window(" messages"))
        assert not window(" paper flight")
        assert "private-fixture" not in store.read_text()

        grab_x, grab_y = origin.x + 46, origin.y + 50
        pointer.move(grab_x, grab_y)
        pointer.button(True)
        try:
            for x, y in ((450, 350), (880, 380)):
                pointer.move(x, y)

                def follows():
                    if dock := window():
                        return (dock.x, dock.y) == (x - 46, y - 50) and not window(" messages")
                    return False

                wait("the float stopped following the pointer or left the badge behind", follows)
            pointer.move(1586, 380)
        finally:
            pointer.button(False)

        def snapped():
            if dock := window():
                return dock if dock.x + dock.w == 1597 and window(" messages") else None
            return None

        docked = wait("the float did not snap right with its message badge", snapped)
        wait("the right edge was not persisted", lambda: json.loads(prefs.read_text())["Edge"] == 1)
        assert badge.x != window(" messages").x

        stop()
        app = start()
        reopened = wait("the right edge was not restored", window)
        assert reopened[1:] == docked[1:], (reopened, docked)
        badge = wait("the unread message did not survive restart", lambda: window(" messages"))
        assert not window(" paper flight"), "restart replayed the paper flight"
        pointer.move(badge.x + 42, badge.y + 42)
        panel = wait("the message panel did not open", lambda: window(" inbox"))
        assert panel.h == 192, panel
        def open_then_dismiss(panel, thread_id):
            route.unlink(missing_ok=True)
            for _ in range(2):
                pointer.click(panel.x + 130, panel.y + 121)
                time.sleep(.05)
            wait("double-click did not return to the Codex turn", route.exists)
            assert route.read_text().splitlines() == [f"codex://threads/{thread_id}"]
            pointer.click(panel.x + panel.w - 58, panel.y + 121)
            wait("single-row dismissal did not remove the message and badge",
                 lambda: not window(" messages") and not store.exists())

        open_then_dismiss(panel, thread)
        finished = "78654321-1234-5678-abcd-123456789012"
        stop_event = json.dumps({"hook_event_name": "Stop", "session_id": finished,
                                 "last_assistant_message": "private-fixture"})
        callback = sp.run([exe, "-completed", "codex", "-hook"], env=env, input=stop_event,
                          text=True, capture_output=True, check=True, timeout=4)
        assert not callback.stdout and not callback.stderr, callback
        badge = wait("Codex Stop hook did not send a notice", lambda: window(" messages"))
        assert json.loads(store.read_text()) == [{"agent": 1, "tone": 0, "thread": finished}]
        pointer.move(badge.x + 42, badge.y + 42)
        panel = wait("completion panel did not open", lambda: window(" inbox"))
        open_then_dismiss(panel, finished)

        pointer.move(reopened.x + 46, reopened.y + 50)
        peek = wait("hover did not open the quota details", lambda: window(" quota details"))
        pointer.move(reopened.x + 46, reopened.y + 142)

        def second_peek():
            if details := window(" quota details"):
                return details if details.xid == peek.xid and details.y == peek.y + 92 else None
            return None

        peek = wait("the quota details did not follow the second mark", second_peek)
        pointer.click(peek.x + peek.w - 20, peek.y + peek.h // 2)
        wait("gear did not open the standalone settings window", lambda: window(" Settings"))
        print("X11: duplicate guard, flight, drag, edge snap, inbox restore, exact route, dismissal and hover settings passed.")
    finally:
        stop()
        pointer.close()


def main(exe):
    with tempfile.TemporaryDirectory(prefix="balance-x11-") as name:
        home = Path(name)
        with (home / "xvfb.log").open("w+b") as xvfb_log, (home / "hud.log").open("w+b") as hud_log:
            server = sp.Popen(
                [os.environ.get("BALANCE_HUD_XVFB", "Xvfb"), "-displayfd", "1",
                 "-screen", "0", "1600x960x24", "-nolisten", "tcp"],
                stdout=sp.PIPE, stderr=xvfb_log,
            )
            try:
                with selectors.DefaultSelector() as ready:
                    ready.register(server.stdout, selectors.EVENT_READ)
                    if not ready.select(timeout=5):
                        raise TimeoutError("Xvfb did not announce a private display")
                number = server.stdout.readline().strip()
                if not number.isdigit():
                    raise RuntimeError("Xvfb did not start")
                smoke(exe, home, ":" + number.decode(), hud_log)
            except Exception:
                for log in (xvfb_log, hud_log):
                    log.seek(0)
                    sys.stderr.write(log.read().decode(errors="replace"))
                raise
            finally:
                server.terminate()
                server.wait(timeout=5)


if __name__ == "__main__":
    main(Path(sys.argv[1]).resolve())
