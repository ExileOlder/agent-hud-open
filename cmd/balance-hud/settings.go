package main

import (
	"fmt"
	"github.com/ExileOlder/agent-hud-open/internal/agentactivity"
	"github.com/ExileOlder/agent-hud-open/internal/codexnotice"
	"sort"
	"strings"

	"github.com/ExileOlder/agent-hud-open/internal/quota"
)

type settingsTools struct {
	codex, agy, claude  string
	notices             <-chan codexnotice.Event
	antigravityActivity <-chan map[string]agentactivity.Tool
	antigravityWaits    <-chan []agentactivity.AntigravityWait
	deepSeekActivity    <-chan agentactivity.DeepSeekStatus
}
type settingsItem struct{ title, value string }
type panelSettings struct {
	page    int
	message string
	raw     []result
}

func (s *panelSettings) items(p placement, rows []row) []settingsItem {
	if s.page == 0 {
		return []settingsItem{
			{"Edge", [...]string{"Top", "Right", "Bottom", "Left"}[p.Edge]},
			{"Show percentages", onOff(p.Numbers)},
			{"Character animation", onOff(!p.ReduceMotion)},
			{"Refresh balances", "Refresh"},
		}
	}
	status := func(i int) string {
		if rows[i].Stale {
			return "Last reading · refresh unavailable"
		}
		if rows[i].Percent < 0 {
			return "No reading"
		}
		return "Connected · " + rows[i].Number
	}
	identity := func(i int) string {
		if rows[i].Identity == "" {
			return "Not selected"
		}
		return rows[i].Identity
	}
	claudeStatus := "Sign in"
	if rows[4].Percent >= 0 {
		claudeStatus = status(4)
	}
	return []settingsItem{
		{"Codex · sign in or switch account", status(0)},
		{"Antigravity · first position", identity(1)},
		{"Antigravity · second position", identity(2)},
		{"Antigravity · authorize another account", "Authorize in browser"},
		{deepSeekDisplayName + " · connect subscription", status(3)},
		{"Claude Code · show in HUD", onOff(p.ShowClaude)},
		{"Claude Code · sign in", claudeStatus},
		{"Codex · approval reminders", "Set up"},
	}
}

func onOff(on bool) string {
	if on {
		return "On"
	}
	return "Off"
}

func agyPool(results []result) []quota.Account {
	for _, r := range results {
		if r.Provider == "antigravity" {
			return r.Accounts
		}
	}
	return nil
}

// A manually selected account stays with its position even if it is absent on
// a later read. Automatic positions only fill from accounts not already pinned.
func selectedAccounts(pool []quota.Account, preferred [2]string) []quota.Account {
	rows := make([]quota.Account, 2)
	used := map[string]bool{}
	for i, name := range preferred {
		if name == "" {
			continue
		}
		rows[i].Label = name
		used[strings.ToLower(name)] = true
		for _, a := range pool {
			if strings.EqualFold(name, a.Label) {
				rows[i] = a
				break
			}
		}
	}
	for i := range rows {
		if rows[i].Label != "" {
			continue
		}
		for _, a := range pool {
			key := strings.ToLower(a.Label)
			if !used[key] {
				rows[i] = a
				used[key] = true
				break
			}
		}
	}
	return rows
}

func settingsResults(raw []result, p placement) []result {
	out := append([]result(nil), raw...)
	for i, r := range out {
		if r.Provider == "antigravity" && r.Error == "" {
			out[i].Accounts = selectedAccounts(r.Accounts, p.Accounts)
		}
	}
	return out
}

func (s *panelSettings) selectAccount(p *placement, index int, rows []row) bool {
	pool := append([]quota.Account(nil), agyPool(s.raw)...)
	if len(pool) < 2 {
		s.message = "Authorize another account to change this position."
		return false
	}
	sort.Slice(pool, func(i, j int) bool { return strings.ToLower(pool[i].Label) < strings.ToLower(pool[j].Label) })
	current, other := rows[index+1].Identity, rows[2-index].Identity
	next := pool[0].Label
	for i, a := range pool {
		if strings.EqualFold(a.Label, current) {
			next = pool[(i+1)%len(pool)].Label
			break
		}
	}
	p.Accounts[index], p.Accounts[1-index] = next, other
	if strings.EqualFold(next, other) {
		p.Accounts[1-index] = current
	}
	s.message = "Positions saved. Click a position again to select another account."
	return true
}

func (s *panelSettings) apply(p *placement, index int, rows []row, t settingsTools) (changed, refresh bool) {
	s.message = ""
	if s.page == 0 {
		switch index {
		case 0:
			p.Edge = (p.Edge + 1) % 4
		case 1:
			p.Numbers = !p.Numbers
		case 2:
			p.ReduceMotion = !p.ReduceMotion
		case 3:
			s.message = "Refreshing balances…"
			return false, true
		default:
			return false, false
		}
		return true, false
	}
	switch index {
	case 0:
		s.message = launchSettingsCommand(t.codex, "login")
	case 1, 2:
		return s.selectAccount(p, index-1, rows), false
	case 3:
		s.message = launchSettingsCommand(t.agy, "login", "--monitor-only")
	case 4:
		s.message = launchVibeSetup()
	case 5:
		p.ShowClaude = !p.ShowClaude
		if p.ShowClaude {
			s.message = "Claude added. Sign in to Claude Code if it has no reading."
		}
		return true, p.ShowClaude
	case 6:
		s.message = launchSettingsCommand(t.claude, "auth", "login")
	case 7:
		s.message = setUpCodexApprovalHook()
	}
	return false, false
}

func (s *panelSettings) help() string {
	if s.message != "" {
		return s.message
	}
	if s.page == 1 {
		return "Click a position to switch to the next authorized account."
	}
	return "Changes are saved automatically for this device."
}

func launchSettingsCommand(exe string, args ...string) string {
	if exe == "" {
		return "Client executable is unavailable."
	}
	if err := openClientTerminal(exe, args...); err != nil {
		return fmt.Sprintf("Could not open sign-in: %s", err)
	}
	return "Finish sign-in, then refresh balances from Display."
}
