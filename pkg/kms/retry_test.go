package kms

import (
	"context"
	"errors"
	"testing"
	"time"
)

// The point of the bound is that the caller is delayed by at most the limit.
// A read that never answers has to end at it, not merely be asked to.
func TestRetry_EndsAtTheLimit(t *testing.T) {
	const limit = 300 * time.Millisecond

	start := time.Now()
	err := Retry(limit, func(context.Context) error { return errors.New("no") })
	took := time.Since(start)

	if err == nil {
		t.Fatal("Retry returned no error for a read that never answered")
	}
	if took < limit/2 {
		t.Errorf("Retry gave up after %v; it did not retry", took)
	}
	if took > limit+100*time.Millisecond {
		t.Errorf("Retry took %v, limit is %v", took, limit)
	}
}

// An attempt that hangs is the case a per-attempt timeout gets wrong: the last
// attempt starts inside the budget and runs past it. The deadline is one, and
// every attempt is under it, so the hang ends with the bound.
func TestRetry_EndsAtTheLimitWhenAnAttemptHangs(t *testing.T) {
	const limit = 300 * time.Millisecond

	start := time.Now()
	err := Retry(limit, func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})
	took := time.Since(start)

	if err == nil {
		t.Fatal("Retry returned no error for a read that hung")
	}
	if took > limit+100*time.Millisecond {
		t.Errorf("a hanging read ran for %v, limit is %v", took, limit)
	}
}

// It retries, and it stops retrying the moment the read answers.
func TestRetry_StopsOnTheFirstAnswer(t *testing.T) {
	tries := 0
	err := Retry(5*time.Second, func(context.Context) error {
		tries++
		if tries < 3 {
			return errors.New("not yet")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Retry = %v, want nil", err)
	}
	if tries != 3 {
		t.Errorf("read was called %d times, want 3", tries)
	}
}

// The error the caller sees is the last one the read gave, so a log says what
// actually happened rather than that time ran out.
func TestRetry_ReturnsTheLastError(t *testing.T) {
	last := errors.New("kms: get acme-seal returned 403")
	err := Retry(200*time.Millisecond, func(context.Context) error { return last })
	if !errors.Is(err, last) {
		t.Errorf("Retry = %v, want the read's own error", err)
	}
}
