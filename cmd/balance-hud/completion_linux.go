package main

import (
	"context"
	"os/exec"
)

func notifySystemCompletion(ctx context.Context, id int) error {
	return exec.CommandContext(ctx, "notify-send", "--app-name=Balance HUD", "--expire-time=5000", agentNames[id], noticeCopy(false)).Run()
}
