package codexnotice

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

var sqlite = syscall.NewLazyDLL("winsqlite3.dll")
var sqliteOpen = sqlite.NewProc("sqlite3_open_v2")
var sqliteClose = sqlite.NewProc("sqlite3_close")
var sqlitePrepare = sqlite.NewProc("sqlite3_prepare_v2")
var sqliteFinalize = sqlite.NewProc("sqlite3_finalize")
var sqliteStep = sqlite.NewProc("sqlite3_step")
var sqliteText = sqlite.NewProc("sqlite3_column_text")
var sqliteBytes = sqlite.NewProc("sqlite3_column_bytes")
var sqliteInt = sqlite.NewProc("sqlite3_column_int")
var moveMemory = syscall.NewLazyDLL("ntdll.dll").NewProc("RtlMoveMemory")

func read(path string) ([]Event, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	if err := sqlite.Load(); err != nil {
		return nil, err
	}
	p, err := syscall.BytePtrFromString(path)
	if err != nil {
		return nil, err
	}
	q, err := syscall.BytePtrFromString(ReadSQL)
	if err != nil {
		return nil, err
	}
	var db, stmt uintptr
	rc, _, _ := sqliteOpen.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&db)), 0x1|0x40000, 0)
	if db != 0 {
		defer sqliteClose.Call(db)
	}
	if rc != 0 {
		return nil, errors.New("cannot read Codex history")
	}
	rc, _, _ = sqlitePrepare.Call(db, uintptr(unsafe.Pointer(q)), ^uintptr(0), uintptr(unsafe.Pointer(&stmt)), 0)
	if rc != 0 {
		return nil, errors.New("unsupported Codex history format")
	}
	defer sqliteFinalize.Call(stmt)
	col := func(i uintptr) string {
		p, _, _ := sqliteText.Call(stmt, i)
		n, _, _ := sqliteBytes.Call(stmt, i)
		if p == 0 || n == 0 {
			return ""
		}
		out := make([]byte, n)
		moveMemory.Call(uintptr(unsafe.Pointer(&out[0])), p, n)
		return string(out)
	}
	var out []Event
	rc, _, _ = sqliteStep.Call(stmt)
	for rc == 100 {
		v, _, _ := sqliteInt.Call(stmt, 2)
		out = append(out, projection(col(0), col(1), int(v)))
		rc, _, _ = sqliteStep.Call(stmt)
	}
	if rc != 101 {
		return nil, errors.New("Codex history unavailable")
	}
	return out, nil
}
