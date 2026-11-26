package main

import (
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
func openProvider(slot int, thread string) {
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
