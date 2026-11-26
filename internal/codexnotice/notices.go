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
}

// Question IDs are stable by item and a completed turn is stable by turn ID.
// The JSON predicates run inside SQLite; only IDs and event kinds leave it.
const ReadSQL = `
SELECT thread_id, 'done:'||turn_id, 0
FROM thread_turns
WHERE status='completed' AND completed_at > CAST(strftime('%s','now') AS INTEGER)-120
  AND (error_json IS NULL OR error_json='null')
UNION ALL
SELECT a.thread_id, 'question:'||a.item_id, 1
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
`

func Watch(ctx context.Context, home string) <-chan Event {
	out := make(chan Event, 16)
	path := filepath.Join(home, "thread_history_1.sqlite")
	go func() {
		defer close(out)
		ticker := time.NewTicker(1250 * time.Millisecond)
		defer ticker.Stop()
		seen := map[string]bool{}
		initial := true
		for {
			rows, err := read(path)
			if err == nil {
				next := make(map[string]bool, len(rows))
				for _, event := range rows {
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
				seen, initial = next, false
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
