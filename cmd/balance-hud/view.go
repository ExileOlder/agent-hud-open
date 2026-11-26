package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type row struct {
	Mark, Number, Details string
	Identity              string
	Percent               int // -1 is unknown
	Stale                 bool
}

func updateView(previous []row, results []result) []row {
	next := view(results)
	if len(previous) != len(next) {
		return next
	}
	for i := range next {
		old, n := previous[i], &next[i]
		if n.Percent >= 0 || old.Percent < 0 {
			continue
		}
		// Do not transfer one account's reading to another account.
		if n.Identity != "" && !strings.EqualFold(n.Identity, old.Identity) {
			continue
		}
		provider := map[int]string{0: "codex", 1: "antigravity", 2: "antigravity", 3: "mistral"}[i]
		keep := n.Identity != ""
		for _, r := range results {
			if r.Provider == provider && r.Error != "" {
				keep = true
			}
		}
		if !keep {
			continue
		}
		n.Identity, n.Percent, n.Number, n.Stale = old.Identity, old.Percent, fmt.Sprintf("%d%%*", old.Percent), true
		detail := old.Details
		if k := strings.Index(detail, "\nLast reading —"); k >= 0 {
			detail = detail[:k]
		}
		n.Details = detail + "\nLast reading — refresh unavailable"
	}
	return next
}

func view(results []result) []row {
	rows := []row{
		{Mark: "C", Number: "--", Details: "Codex: waiting for quota", Percent: -1},
		{Mark: "A1", Number: "--", Details: "Antigravity account 1: waiting for quota", Percent: -1},
		{Mark: "A2", Number: "--", Details: "Antigravity account 2: waiting for quota", Percent: -1},
		{Mark: "M", Number: "--", Details: "Mistral Vibe: waiting for quota", Percent: -1},
	}
	for _, r := range results {
		start, count := 0, 1
		switch r.Provider {
		case "antigravity":
			start, count = 1, 2
		case "mistral":
			start = 3
		case "codex":
		default:
			continue
		}
		for j := 0; j < count; j++ {
			dst := &rows[start+j]
			if r.Error != "" {
				dst.Details = r.Provider + ": " + r.Error
				continue
			}
			if j >= len(r.Accounts) {
				dst.Details = r.Provider + ": account not authorized"
				continue
			}
			a := r.Accounts[j]
			dst.Identity = a.Label
			lines := []string{a.Label}
			if a.Balance != "" {
				lines = append(lines, a.Balance)
			}
			low := 101.0
			for _, w := range a.Windows {
				if w.Remaining < low {
					low = w.Remaining
				}
				line := fmt.Sprintf("%s: %.0f%% remaining", w.Name, w.Remaining)
				if t, e := time.Parse(time.RFC3339, w.Reset); e == nil {
					line += " · resets " + t.Local().Format("Jan 2 15:04")
				}
				lines = append(lines, line)
			}
			if low <= 100 {
				dst.Percent = int(math.Round(low))
				dst.Number = fmt.Sprintf("%d%%", dst.Percent)
			} else {
				lines = append(lines, "Quota unavailable")
			}
			dst.Details = strings.Join(lines, "\n")
		}
	}
	return rows
}

type placement struct {
	Edge    int
	Offset  float64
	Numbers bool
}

func settingsPath() string {
	dir, e := os.UserConfigDir()
	if e != nil {
		return ""
	}
	return filepath.Join(dir, "BalanceHUD", "appearance.json")
}
func loadPlacement() placement {
	p := placement{Edge: 1, Offset: .5, Numbers: false}
	if data, e := os.ReadFile(settingsPath()); e == nil {
		var saved placement
		if json.Unmarshal(data, &saved) == nil && saved.Edge >= 0 && saved.Edge <= 3 && saved.Offset >= 0 && saved.Offset <= 1 {
			p = saved
		}
	}
	return p
}
func (p placement) save() error {
	path := settingsPath()
	if path == "" {
		return fmt.Errorf("configuration directory unavailable")
	}
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	b, e := json.Marshal(p)
	if e != nil {
		return e
	}
	return os.WriteFile(path, b, 0600)
}
