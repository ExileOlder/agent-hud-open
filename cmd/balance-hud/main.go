package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ExileOlder/agent-hud-open/internal/agentactivity"
	"github.com/ExileOlder/agent-hud-open/internal/codexnotice"
	"github.com/ExileOlder/agent-hud-open/internal/quota"
)

type result struct {
	Provider string          `json:"provider"`
	Accounts []quota.Account `json:"accounts,omitempty"`
	Error    string          `json:"error,omitempty"`
}

const bundledMonitorFlag = "--antigravity-helper"

// Set only in portable builds; a source build still uses an external monitor.
var bundledMonitor func([]string) int

func main() {
	if len(os.Args) > 1 && os.Args[1] == bundledMonitorFlag && bundledMonitor != nil {
		os.Exit(bundledMonitor(os.Args[2:]))
	}
	codex := flag.String("codex", "codex", "Codex CLI executable")
	agy := flag.String("antigravity", "", "override the Antigravity monitor executable")
	claude := flag.String("claude", claudeExecutable(), "Claude Code CLI executable")
	desktop := flag.Bool("desktop", true, "open native quota HUD")
	setup := flag.Bool("setup-mistral", false, "save a Mistral Vibe key in the Linux credential store")
	completion := flag.String("completed", "", "show completion reminder (codex, antigravity, mistral, claude, agent)")
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
	signInMonitor := *agy
	var monitorPrefix []string
	quotaMonitor := signInMonitor
	if signInMonitor == "" {
		signInMonitor = monitorExecutable()
		quotaMonitor = signInMonitor
		if bundledMonitor != nil {
			if self, err := os.Executable(); err == nil {
				quotaMonitor, monitorPrefix = self, []string{bundledMonitorFlag}
			}
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fetch := func() <-chan result {
		name := ""
		if loadPlacement().ShowClaude {
			name = *claude
		}
		return fetchQuotas(ctx, *codex, quotaMonitor, name, monitorPrefix...)
	}
	if *desktop {
		var notices <-chan codexnotice.Event
		home := os.Getenv("CODEX_HOME")
		if home == "" {
			if dir, err := os.UserHomeDir(); err == nil {
				home = filepath.Join(dir, ".codex")
			}
		}
		if home != "" {
			notices = codexnotice.Watch(ctx, home)
		}
		profiles := loadAntigravityProfiles()
		if err := runDesktop(fetch, settingsTools{
			codex: *codex, agy: signInMonitor, claude: *claude, notices: notices,
			antigravityActivity: agentactivity.WatchAntigravity(ctx, profiles),
			antigravityWaits:    agentactivity.WatchAntigravityWaits(ctx, profiles),
			deepSeekActivity:    agentactivity.WatchDeepSeek(ctx, deepSeekHome()),
		}); err != nil {
			println(err.Error())
			os.Exit(1)
		}
		return
	}
	if json.NewEncoder(os.Stdout).Encode(collectQuotas(fetch())) != nil {
		os.Exit(1)
	}
}

func fetchQuotas(parent context.Context, codex, agy, claude string, monitorPrefix ...string) <-chan result {
	sources := []quotaSource{
		{"codex", func(ctx context.Context) ([]quota.Account, error) {
			row, err := quota.Codex(ctx, codex)
			if err != nil {
				return nil, err
			}
			return []quota.Account{row}, nil
		}},
		{"antigravity", func(ctx context.Context) ([]quota.Account, error) {
			return quota.Antigravity(ctx, agy, monitorPrefix...)
		}},
		{"mistral", func(ctx context.Context) ([]quota.Account, error) {
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
		}},
	}
	if claude != "" {
		sources = append(sources, quotaSource{"claude", func(ctx context.Context) ([]quota.Account, error) {
			row, err := quota.Claude(ctx, claude)
			if err != nil {
				return nil, err
			}
			return []quota.Account{row}, nil
		}})
	}
	return refreshQuotas(parent, sources)
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

func claudeExecutable() string {
	if exe, err := exec.LookPath("claude"); err == nil {
		return exe
	}
	if home, err := os.UserHomeDir(); runtime.GOOS == "linux" && err == nil {
		local := filepath.Join(home, "AI", "Claude", "App", "claude")
		if info, err := os.Stat(local); err == nil && info.Mode().IsRegular() {
			return local
		}
	}
	return "claude"
}
