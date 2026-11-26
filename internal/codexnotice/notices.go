// Package codexnotice watches Codex's local history projection for event
// metadata. It never returns questions, answers, or conversation text.
package codexnotice

import (
	"context"
	"github.com/ExileOlder/agent-hud-open/internal/agentactivity"
	"path/filepath"
	"time"
)

type Event struct {
	Thread   string
	ID       string
	Question bool
	Activity bool
	Working  bool
	Pending  bool
	Tool     agentactivity.Tool
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
),
active AS (
 SELECT t.thread_id,t.turn_id
 FROM thread_turns t
 WHERE t.status='inProgress'
  AND max(t.started_at, coalesce((
   SELECT max(coalesce(i.completed_at_ms, i.started_at_ms, i.created_at_ms))/1000
   FROM thread_items i WHERE i.thread_id=t.thread_id AND i.turn_id=t.turn_id
  ),0)) > CAST(strftime('%s','now') AS INTEGER)-1800
  AND NOT EXISTS (SELECT 1 FROM pending p WHERE p.thread_id=t.thread_id AND p.turn_id=t.turn_id)
),
tool AS (
 SELECT CASE
  WHEN i.item_type IN ('imageView','webSearch') THEN 3
  WHEN i.item_type='fileChange' THEN 4
  WHEN i.item_type='commandExecution' THEN CASE
    WHEN EXISTS (
      SELECT 1 FROM json_each(i.item_json,'$.commandActions') j
      WHERE json_extract(j.value,'$.type') IN ('read','listFiles','search')
    ) THEN 3
    ELSE 5 END
  ELSE 2 END AS kind
 FROM thread_items i JOIN active a ON i.thread_id=a.thread_id AND i.turn_id=a.turn_id
 WHERE i.item_type IN ('imageView','webSearch','fileChange','commandExecution')
  AND (
   coalesce(i.completed_at_ms,i.started_at_ms,i.created_at_ms)/1000 > CAST(strftime('%s','now') AS INTEGER)-10
   OR json_extract(i.item_json,'$.status')='inProgress'
  )
 ORDER BY coalesce(i.started_at_ms,i.created_at_ms) DESC
 LIMIT 1
)
SELECT thread_id, 'done:'||turn_id, 0
FROM thread_turns
WHERE status='completed' AND completed_at > CAST(strftime('%s','now') AS INTEGER)-120
  AND (error_json IS NULL OR error_json='null')
UNION ALL
SELECT thread_id, 'question:'||item_id, 1 FROM pending
UNION ALL
SELECT '', '', COALESCE((SELECT kind FROM tool),2) WHERE EXISTS(SELECT 1 FROM active)
`

func projection(thread, id string, kind int) Event {
	event := Event{Thread: thread, ID: id, Question: kind == 1, Activity: kind >= 2, Working: kind >= 2}
	if event.Activity {
		event.Tool = agentactivity.Tool(kind - 2)
	}
	return event
}

func Watch(ctx context.Context, home string) <-chan Event {
	out := make(chan Event, 16)
	path := filepath.Join(home, "thread_history_1.sqlite")
	go func() {
		defer close(out)
		ticker := time.NewTicker(1250 * time.Millisecond)
		defer ticker.Stop()
		seen := map[string]bool{}
		initial := true
		lastWork := Event{Activity: true}
		for {
			rows, err := read(path)
			if err == nil {
				next := make(map[string]bool, len(rows))
				work := Event{Activity: true}
				pending := false
				for _, event := range rows {
					if event.Activity {
						work = event
						continue
					}
					pending = pending || event.Question
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
				work.Pending = pending
				if work != lastWork {
					select {
					case out <- work:
					case <-ctx.Done():
						return
					}
				}
				seen, initial, lastWork = next, false, work
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
