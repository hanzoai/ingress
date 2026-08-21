package kms

import (
	"context"
	"time"
)

// Retry calls read until it answers, or until limit elapses.
//
// A KMS that answers on the second try is the common case — a pod that starts
// before its network is ready, a rollout on the KMS side — and a read that
// gives up on the first answer turns those into a start that does not happen.
// A read that never gives up turns them into a start that never finishes.
// So: retry, and bound it.
//
// The bound is one deadline, taken once, and it covers the attempt in flight as
// well as the waits between attempts. Every attempt runs under a context
// derived from that deadline, so a read that hangs is cut at the bound rather
// than extending past it, and the loop stops as soon as the next wait would
// cross it. Whatever the caller does with the error, it does within limit.
//
// The wait doubles from half a second and is trimmed to what is left of the
// budget, so the whole budget is spent trying rather than abandoned because the
// next wait would have been inconvenient.
func Retry(limit time.Duration, read func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	deadline, _ := ctx.Deadline()

	for wait := 500 * time.Millisecond; ; wait *= 2 {
		err := read(ctx)
		if err == nil {
			return nil
		}
		left := time.Until(deadline)
		if left <= 0 {
			return err
		}
		if wait > left {
			wait = left
		}
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return err
		}
	}
}
