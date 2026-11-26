// Package codexnotice watches Codex's local history projection for event
// metadata. It never returns questions, answers, or conversation text.
package codexnotice

import (
	"context"
	"path/filepath"
	"time"
)

type Event struct {
	Thread   string
	ID       string
	Question bool
	Activity bool
	Working  bool
}

// Question IDs are stable by item and a completed turn is stable by turn ID.
// The JSON predicates run inside SQLite; only IDs and event kinds leave it.
// Work follows the original HUD's 30-minute abandonment rule, measured
// from the latest projected activity, not from the start of a long turn.
const ReadSQL = `
WITH pending AS (
 SELECT a.thread_id, a.turn_id, a.item_id
 FROM thread_turns t JOIN thread_items a ON t.thread_id=a.thread_id AND t.turn_id=a.turn_id
 WHERE t.status='inProgress'
  AND t.started_at > CAST(strftime('%s','now') AS INTEGER)-86400
  AND a.item_type='agentMessage' AND json_array_length(a.item_json,'$.questions')>0
  AND NOT EXISTS (
   SELECT 1 FROM thread_items u
   WHERE u.thread_id=a.thread_id AND u.turn_id=a.turn_id
    AND u.item_type='userMessage'
    AND u.rollout_ordinal>a.rollout_ordinal
    AND instr(json_extract(u.item_json,'$.content[0].text'),'<send_user_message_question_reply>')>0
    AND instr(json_extract(u.item_json,'$.content[0].text'),a.item_id)>0
  )
)
SELECT thread_id, 'done:'||turn_id, 0
FROM thread_turns
WHERE status='completed' AND completed_at > CAST(strftime('%s','now') AS INTEGER)-120
  AND (error_json IS NULL OR error_json='null')
UNION ALL
SELECT thread_id, 'question:'||item_id, 1 FROM pending
UNION ALL
SELECT '', '', 2
WHERE EXISTS (
 SELECT 1 FROM thread_turns t
 WHERE t.status='inProgress'
  AND max(t.started_at, coalesce((
   SELECT max(coalesce(i.completed_at_ms, i.started_at_ms, i.created_at_ms))/1000
   FROM thread_items i WHERE i.thread_id=t.thread_id AND i.turn_id=t.turn_id
  ),0)) > CAST(strftime('%s','now') AS INTEGER)-1800
  AND NOT EXISTS (SELECT 1 FROM pending p WHERE p.thread_id=t.thread_id AND p.turn_id=t.turn_id)
)
`

func projection(thread, id string, kind int) Event {
	return Event{Thread: thread, ID: id, Question: kind == 1, Activity: kind == 2, Working: kind == 2}
}

func Watch(ctx context.Context, home string) <-chan Event {
	out := make(chan Event, 16)
	path := filepath.Join(home, "thread_history_1.sqlite")
	go func() {
		defer close(out)
		ticker := time.NewTicker(1250 * time.Millisecond)
		defer ticker.Stop()
		seen := map[string]bool{}
		initial, wasWorking := true, false
		for {
			rows, err := read(path)
			if err == nil {
				next := make(map[string]bool, len(rows))
				working := false
				for _, event := range rows {
					if event.Activity {
						working = true
						continue
					}
					key := event.Thread + "/" + event.ID
					next[key] = true
					if seen[key] || initial && !event.Question {
						continue
					}
					select {
					case out <- event:
					case <-ctx.Done():
						return
					}
				}
				if working != wasWorking {
					select {
					case out <- Event{Activity: true, Working: working}:
					case <-ctx.Done():
						return
					}
				}
				seen, initial, wasWorking = next, false, working
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return out
}
