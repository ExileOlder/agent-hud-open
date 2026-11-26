//go:build linux && !cgo

package main

import "context"

func notifyCompletion(ctx context.Context, id int) error {
	return notifySystemCompletion(ctx, id)
}
