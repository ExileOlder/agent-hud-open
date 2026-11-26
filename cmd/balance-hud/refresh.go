package main

import (
	"context"
	"sync"
	"time"

	"github.com/ExileOlder/agent-hud-open/internal/quota"
)

type quotaSource struct {
	name string
	read func(context.Context) ([]quota.Account, error)
}

// Each reading has the same deadline as before, but an unfinished provider
// never holds a completed reading. The buffer also lets workers return when
// a native window closes without consuming the rest of the batch.
func refreshQuotas(parent context.Context, sources []quotaSource) <-chan result {
	ready := make(chan result, len(sources))
	var pending sync.WaitGroup
	pending.Add(len(sources))
	for _, source := range sources {
		go func() {
			defer pending.Done()
			ctx, cancel := context.WithTimeout(parent, 75*time.Second)
			defer cancel()
			rows, err := source.read(ctx)
			r := result{Provider: source.name, Accounts: rows}
			if err != nil {
				r.Error = err.Error()
			}
			ready <- r
		}()
	}
	go func() {
		pending.Wait()
		close(ready)
	}()
	return ready
}

func mergeQuota(raw []result, incoming result) []result {
	for i := range raw {
		if raw[i].Provider == incoming.Provider {
			raw[i] = incoming
			return raw
		}
	}
	return append(raw, incoming)
}

// Preserve the existing command-line output order, independent of latency.
func collectQuotas(updates <-chan result) []result {
	got := make(map[string]result, 4)
	for r := range updates {
		got[r.Provider] = r
	}
	rows := make([]result, 0, len(got))
	for _, name := range [...]string{"codex", "antigravity", "mistral", "claude"} {
		if r, ok := got[name]; ok {
			rows = append(rows, r)
		}
	}
	return rows
}
