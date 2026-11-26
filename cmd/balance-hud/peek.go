package main

import (
	"fmt"
	"strings"

	"github.com/ExileOlder/agent-hud-open/internal/quota"
)

// The Simple dock's AgentPeek groups each short label with its reading, with
// the Antigravity account first. Keep values separate for native font/color.
type peekItem struct {
	Label, Value string
	Tone         int // neutral, good, warning, low
}

func quotaTone(percent float64) int {
	switch {
	case percent <= 15:
		return 3
	case percent <= 30:
		return 2
	default:
		return 1
	}
}

func shortWindow(name string) string {
	switch strings.ToLower(name) {
	case "gemini models weekly":
		return "Gemini 7d"
	case "gemini models 5h":
		return "Gemini 5h"
	case "claude and gpt models weekly":
		return "3rd-party 7d"
	case "claude and gpt models 5h":
		return "3rd-party 5h"
	case "7d", "weekly":
		return "Weekly limit"
	case "5h":
		return "5h limit"
	default:
		return name
	}
}

func accountPeek(provider string, a quota.Account) []peekItem {
	if provider == "mistral" && a.Balance != "" && len(a.Windows) != 0 {
		b := a.Balance
		if amount, total, ok := strings.Cut(strings.TrimSuffix(b, " USD"), " / "); ok {
			b = "$" + amount + " / $" + total
		}
		remaining := a.Windows[0].Remaining
		return []peekItem{{"Mistral", fmt.Sprintf("%s (%.2f%%)", b, remaining), quotaTone(remaining)}}
	}
	items := []peekItem{}
	if provider == "antigravity" {
		items = append(items, peekItem{"Account", a.Label, 0})
	}
	seen := map[string]bool{}
	for _, w := range a.Windows {
		name := shortWindow(w.Name)
		if !seen[name] {
			seen[name] = true
			items = append(items, peekItem{name, fmt.Sprintf("%.0f%%", w.Remaining), quotaTone(w.Remaining)})
		}
	}
	return items
}

func (r row) peekLine(slot int) []peekItem {
	if len(r.Peek) != 0 {
		if r.Stale {
			items := append([]peekItem(nil), r.Peek...)
			for i := range items {
				items[i].Tone = 0
			}
			return append(items, peekItem{"Quota", "Last reading", 0})
		}
		return r.Peek
	}
	if slot == 4 {
		return []peekItem{{"Claude Code", "--", 0}}
	}
	if slot == 3 {
		return []peekItem{{"Mistral", "--", 0}}
	}
	return []peekItem{{"Quota", "--", 0}}
}
