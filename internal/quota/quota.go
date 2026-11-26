// Package quota reads subscription limits without sending model prompts.
package quota

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

type Window struct {
	Name      string
	Remaining float64
	Reset     string
}
type Account struct {
	Label   string
	Windows []Window
	Balance string
}

func valid(p float64) bool { return p >= 0 && p <= 100 }

func Mistral(ctx context.Context, key string) (Account, error) {
	if strings.TrimSpace(key) == "" {
		return Account{}, errors.New("Mistral Vibe key is not configured")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.mistral.ai/v1/billing/budget", nil)
	if err != nil {
		return Account{}, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")
	client := http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		return Account{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return Account{}, fmt.Errorf("Mistral budget: HTTP %d", res.StatusCode)
	}
	return parseMistral(io.LimitReader(res.Body, 1<<20))
}
func parseMistral(r io.Reader) (Account, error) {
	var d struct {
		Budget *struct {
			Initial  float64  `json:"initial_budget"`
			Used     *float64 `json:"usage_percentage"`
			Currency string
			Reset    string `json:"reset_at"`
		} `json:"vibe_budget"`
	}
	if err := json.NewDecoder(r).Decode(&d); err != nil {
		return Account{}, err
	}
	b := d.Budget
	if b == nil || b.Used == nil || !valid(*b.Used) || b.Initial <= 0 || b.Currency == "" {
		return Account{}, errors.New("invalid Vibe budget")
	}
	rem := 100 - *b.Used
	return Account{Label: "Mistral Vibe", Windows: []Window{{"Subscription", rem, b.Reset}}, Balance: fmt.Sprintf("%.2f / %.2f %s", b.Initial*rem/100, b.Initial, b.Currency)}, nil
}

func Codex(ctx context.Context, executable string) (Account, error) {
	cmd := exec.CommandContext(ctx, executable, "app-server", "--listen", "stdio://")
	configureProcess(cmd)
	in, err := cmd.StdinPipe()
	if err != nil {
		return Account{}, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		in.Close()
		return Account{}, err
	}
	if err = cmd.Start(); err != nil {
		in.Close()
		out.Close()
		return Account{}, err
	}
	defer func() { in.Close(); cmd.Process.Kill(); cmd.Wait() }()
	enc := json.NewEncoder(in)
	if err = enc.Encode(map[string]any{"id": 1, "method": "initialize", "params": map[string]any{"clientInfo": map[string]string{"name": "balance_hud", "title": "Balance HUD", "version": "0.1.0"}}}); err != nil {
		return Account{}, err
	}
	scan := bufio.NewScanner(out)
	scan.Buffer(make([]byte, 4096), 1<<20)
	for scan.Scan() {
		var msg struct {
			ID     int
			Result json.RawMessage
			Error  json.RawMessage
		}
		if json.Unmarshal(scan.Bytes(), &msg) != nil {
			continue
		}
		if msg.ID != 1 && msg.ID != 2 {
			continue
		}
		if len(msg.Error) > 0 && string(msg.Error) != "null" {
			return Account{}, errors.New("Codex app-server rejected quota request")
		}
		if msg.ID == 2 {
			return parseCodex(msg.Result)
		}
		if err = enc.Encode(map[string]string{"method": "initialized"}); err != nil {
			return Account{}, err
		}
		if err = enc.Encode(map[string]any{"id": 2, "method": "account/rateLimits/read"}); err != nil {
			return Account{}, err
		}
	}
	if ctx.Err() != nil {
		return Account{}, ctx.Err()
	}
	if scan.Err() != nil {
		return Account{}, scan.Err()
	}
	return Account{}, errors.New("Codex app-server returned no quota")
}

type limitWindow struct {
	Used    *float64 `json:"usedPercent"`
	Minutes int      `json:"windowDurationMins"`
	Reset   int64    `json:"resetsAt"`
}
type limits struct {
	Primary, Secondary *limitWindow
	Name               string `json:"limitName"`
}

func parseCodex(data []byte) (Account, error) {
	var d struct {
		Limits limits            `json:"rateLimits"`
		ByID   map[string]limits `json:"rateLimitsByLimitId"`
	}
	if err := json.Unmarshal(data, &d); err != nil {
		return Account{}, err
	}
	a := Account{Label: "Codex"}
	selected := d.Limits
	if row, ok := d.ByID["codex"]; ok {
		selected = row
	}
	for _, w := range []*limitWindow{selected.Primary, selected.Secondary} {
		if w == nil || w.Used == nil || !valid(*w.Used) {
			continue
		}
		label := "Limit"
		switch {
		case w.Minutes >= 1440 && w.Minutes%1440 == 0:
			label = fmt.Sprintf("%dd", w.Minutes/1440)
		case w.Minutes >= 60 && w.Minutes%60 == 0:
			label = fmt.Sprintf("%dh", w.Minutes/60)
		case w.Minutes > 0:
			label = fmt.Sprintf("%dm", w.Minutes)
		}
		var reset string
		if w.Reset > 0 {
			reset = time.Unix(w.Reset, 0).Format(time.RFC3339)
		}
		a.Windows = append(a.Windows, Window{label, 100 - *w.Used, reset})
	}
	if len(a.Windows) == 0 {
		return Account{}, errors.New("Codex reported no subscription windows")
	}
	return a, nil
}

// Antigravity uses the separately authorized, read-only agy-auth monitor.
// The process receives no credential through its argument list.
func Antigravity(ctx context.Context, executable string, prefix ...string) ([]Account, error) {
	args := append(prefix[:len(prefix):len(prefix)], "--json", "usage", "--all")
	cmd := exec.CommandContext(ctx, executable, args...)
	configureProcess(cmd)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		out.Close()
		return nil, err
	}
	defer func() { out.Close(); cmd.Process.Kill(); cmd.Wait() }()
	return parseAntigravity(io.LimitReader(out, 1<<20))
}
func parseAntigravity(r io.Reader) ([]Account, error) {
	var d struct {
		Data struct {
			Accounts []struct {
				Email  string
				Groups []struct {
					Name    string
					Buckets []struct {
						Window    string
						Remaining *float64 `json:"remaining_fraction"`
						Reset     string   `json:"reset_time"`
					}
				}
			}
		}
	}
	if err := json.NewDecoder(r).Decode(&d); err != nil {
		return nil, err
	}
	var rows []Account
	seen := map[string]bool{}
	for _, a := range d.Data.Accounts {
		email := strings.TrimSpace(a.Email)
		id := strings.ToLower(email)
		if email == "" || seen[id] {
			continue
		}
		seen[id] = true
		row := Account{Label: email}
		for _, g := range a.Groups {
			for _, b := range g.Buckets {
				if b.Remaining != nil && valid(*b.Remaining*100) {
					row.Windows = append(row.Windows, Window{g.Name + " " + b.Window, *b.Remaining * 100, b.Reset})
				}
			}
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil, errors.New("no authorized Antigravity accounts returned")
	}
	return rows, nil
}
