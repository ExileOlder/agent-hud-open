package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ExileOlder/agent-hud-open/internal/quota"
)

type result struct {
	Provider string          `json:"provider"`
	Accounts []quota.Account `json:"accounts,omitempty"`
	Error    string          `json:"error,omitempty"`
}

func main() {
	codex := flag.String("codex", "codex", "Codex CLI executable")
	agy := flag.String("antigravity", monitorExecutable(), "authorized Antigravity monitor executable")
	desktop := flag.Bool("desktop", true, "open native quota HUD")
	setup := flag.Bool("setup-mistral", false, "save a Mistral Vibe key in the Linux credential store")
	completion := flag.String("completed", "", "show completion reminder (codex, antigravity, mistral, agent)")
	hook := flag.Bool("hook", false, "read completion event from the client's hook stdin")
	flag.Parse()
	if *completion != "" {
		if err := runCompletion(*completion, *hook, os.Stdin, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			if !*hook {
				os.Exit(1)
			}
		}
		return
	}
	if *setup {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if err := setupVibeKey(ctx); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "Mistral Vibe key saved.")
		return
	}
	if os.Getenv("AGY_AUTH_HOME") == "" {
		if dir, err := os.UserConfigDir(); err == nil {
			if err := os.Setenv("AGY_AUTH_HOME", filepath.Join(dir, "BalanceHUD", "Antigravity")); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		}
	}
	fetch := func() []result { return fetchQuotas(*codex, *agy) }
	if *desktop {
		if err := runDesktop(fetch, settingsTools{codex: *codex, agy: *agy}); err != nil {
			println(err.Error())
			os.Exit(1)
		}
		return
	}
	if json.NewEncoder(os.Stdout).Encode(fetch()) != nil {
		os.Exit(1)
	}
}

func fetchQuotas(codex, agy string) []result {
	results := make([]result, 0, 3)
	collect := func(name string, f func(context.Context) ([]quota.Account, error)) {
		ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
		defer cancel()
		rows, err := f(ctx)
		row := result{Provider: name, Accounts: rows}
		if err != nil {
			row.Error = err.Error()
		}
		results = append(results, row)
	}
	collect("codex", func(ctx context.Context) ([]quota.Account, error) {
		row, err := quota.Codex(ctx, codex)
		if err != nil {
			return nil, err
		}
		return []quota.Account{row}, nil
	})
	collect("antigravity", func(ctx context.Context) ([]quota.Account, error) { return quota.Antigravity(ctx, agy) })
	collect("mistral", func(ctx context.Context) ([]quota.Account, error) {
		key := strings.TrimSpace(os.Getenv("MISTRAL_VIBE_API_KEY"))
		if key == "" {
			var err error
			key, err = savedVibeKey(ctx)
			if err != nil {
				return nil, err
			}
		}
		row, err := quota.Mistral(ctx, key)
		if err != nil {
			return nil, err
		}
		return []quota.Account{row}, nil
	})
	return results
}

func monitorExecutable() string {
	name := "agy-auth-monitor"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if exe, err := os.Executable(); err == nil {
		path := filepath.Join(filepath.Dir(exe), name)
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return path
		}
	}
	return name
}
