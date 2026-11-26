package main

import (
	"github.com/ExileOlder/agent-hud-open/internal/agentactivity"
	"os"
	"regexp"
)

var codexThreadID = regexp.MustCompile(`^[[:xdigit:]]{8}-[[:xdigit:]]{4}-[[:xdigit:]]{4}-[[:xdigit:]]{4}-[[:xdigit:]]{12}$`)

func clientRoute(slot int, thread string) string {
	switch slot {
	case 0:
		if codexThreadID.MatchString(thread) {
			return "codex://threads/" + thread
		}
		return "codex://"
	case 1, 2:
		return "antigravity://"
	case 3:
		return "https://chat.mistral.ai/code"
	case 4:
		return "claude://"
	default:
		return ""
	}
}
func openNotice(item mailItem) {
	if item.agent == deepSeekAgent {
		openDeepSeek()
		return
	}
	if item.agent == 3 {
		if err := openClientURI(clientRoute(3, "")); err != nil {
			os.Stderr.WriteString(err.Error() + "\n")
		}
		return
	}
	openProvider(completionSlot(item.agent), item.event.Thread)
}

func openDeepSeek() {
	if err := launchDeepSeek(); err != nil {
		os.Stderr.WriteString("Could not open DeepSeek Harness: " + err.Error() + "\n")
	}
}

func openProvider(slot int, thread string) {
	if slot == 3 && agentactivity.DeepSeekHome(deepSeekHome()) {
		openDeepSeek()
		return
	}
	if route := clientRoute(slot, thread); route != "" {
		if err := openClientURI(route); err != nil {
			os.Stderr.WriteString("Could not open client: " + err.Error() + "\n")
		}
	}
}
func noticeCopy(question bool) string {
	if question {
		return "等你补充一下"
	}
	return "结果可以查看了"
}
