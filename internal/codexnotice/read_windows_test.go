package codexnotice

import (
	"path/filepath"
	"syscall"
	"testing"
	"unsafe"
)

func TestWindowsReadsProjectionThroughSystemSQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "thread_history_1.sqlite")
	p, err := syscall.BytePtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	var db uintptr
	rc, _, _ := sqliteOpen.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&db)), 0x2|0x4, 0)
	if db != 0 {
		defer sqliteClose.Call(db)
	}
	if rc != 0 {
		t.Fatalf("WinSQLite3 create: %d", rc)
	}
	text, err := syscall.BytePtrFromString(`
CREATE TABLE thread_turns(thread_id TEXT, turn_id TEXT, status TEXT, completed_at INTEGER, error_json TEXT, started_at INTEGER);
CREATE TABLE thread_items(thread_id TEXT, turn_id TEXT, item_id TEXT, item_type TEXT, item_json TEXT, rollout_ordinal INTEGER);
INSERT INTO thread_turns VALUES ('done-thread','done','completed',strftime('%s','now'),NULL,0);
INSERT INTO thread_turns VALUES ('question-thread','pending','inProgress',NULL,NULL,strftime('%s','now'));
INSERT INTO thread_items VALUES ('question-thread','pending','question-item','agentMessage','{"questions":[{"title":"private"}]}',1);
`)
	if err != nil {
		t.Fatal(err)
	}
	rc, _, _ = sqlite.NewProc("sqlite3_exec").Call(db, uintptr(unsafe.Pointer(text)), 0, 0, 0)
	if rc != 0 {
		t.Fatalf("WinSQLite3 fixture: %d", rc)
	}
	events, err := read(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 ||
		events[0] != (Event{Thread: "done-thread", ID: "done:done"}) ||
		events[1] != (Event{Thread: "question-thread", ID: "question:question-item", Question: true}) {
		t.Fatalf("wrong event projection: %+v", events)
	}
}
