// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package database

import (
	"context"
	"errors"
	"testing"
)

var errRetryable = errors.New("retryable")
var errPermanent = errors.New("permanent")

func isRetryableErr(err error) bool { return errors.Is(err, errRetryable) }

func TestRetry_SuccessFirstTry(t *testing.T) {
	calls := 0
	err := Retry(context.Background(), isRetryableErr, func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestRetry_RetryableThenSuccess(t *testing.T) {
	calls := 0
	err := Retry(context.Background(), isRetryableErr, func() error {
		calls++
		if calls < 2 {
			return errRetryable
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}

func TestRetry_PermanentErrorShortCircuits(t *testing.T) {
	calls := 0
	err := Retry(context.Background(), isRetryableErr, func() error {
		calls++
		return errPermanent
	})
	if !errors.Is(err, errPermanent) {
		t.Fatalf("expected permanent error to bubble up, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("permanent error must not retry; got %d calls", calls)
	}
}

func TestRetry_ContextCanceledStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already canceled before starting

	calls := 0
	err := Retry(ctx, isRetryableErr, func() error {
		calls++
		return errRetryable
	})
	if err == nil {
		t.Fatalf("expected error when context already canceled")
	}
	// backoff returns the last op error or ctx.Err — either is acceptable; the
	// invariant is that we don't loop forever.
	if calls > 5 {
		t.Fatalf("context-canceled retry should not loop indefinitely; got %d calls", calls)
	}
}
