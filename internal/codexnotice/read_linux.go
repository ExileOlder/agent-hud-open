//go:build cgo

package codexnotice

/*
#cgo pkg-config: sqlite3
#include <sqlite3.h>
#include <stdlib.h>
*/
import "C"

import (
	"errors"
	"os"
	"unsafe"
)

func read(path string) ([]Event, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	p, q := C.CString(path), C.CString(ReadSQL)
	defer C.free(unsafe.Pointer(p))
	defer C.free(unsafe.Pointer(q))
	var db *C.sqlite3
	rc := C.sqlite3_open_v2(p, &db, C.SQLITE_OPEN_READONLY|C.SQLITE_OPEN_PRIVATECACHE, nil)
	if db != nil {
		defer C.sqlite3_close(db)
	}
	if rc != C.SQLITE_OK {
		return nil, errors.New("cannot read Codex history")
	}
	var stmt *C.sqlite3_stmt
	if C.sqlite3_prepare_v2(db, q, -1, &stmt, nil) != C.SQLITE_OK {
		return nil, errors.New("unsupported Codex history format")
	}
	defer C.sqlite3_finalize(stmt)
	var out []Event
	rc = C.sqlite3_step(stmt)
	for rc == C.SQLITE_ROW {
		out = append(out, Event{
			Thread:   C.GoString((*C.char)(unsafe.Pointer(C.sqlite3_column_text(stmt, 0)))),
			ID:       C.GoString((*C.char)(unsafe.Pointer(C.sqlite3_column_text(stmt, 1)))),
			Question: C.sqlite3_column_int(stmt, 2) != 0,
		})
		rc = C.sqlite3_step(stmt)
	}
	if rc != C.SQLITE_DONE {
		return nil, errors.New("Codex history unavailable")
	}
	return out, nil
}
