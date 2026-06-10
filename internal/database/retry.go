// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package database

import (
	"context"

	"github.com/cenkalti/backoff/v4"
)

// Retry executes op with exponential back-off, calling isRetryable to decide
// whether a given error warrants another attempt.
//
// Policy: up to 4 retries, starting at ~500 ms, doubling each time with ±50%
// jitter, capped at 60 s. Any error for which isRetryable returns false is
// wrapped in backoff.Permanent and returned immediately — we must not silently
// retry create/delete side-effects that are not idempotent.
//
// Engine-specific packages (mssql, postgres) call this with their own
// isRetryable classifier (deadlock 1205 / SQLSTATE 40P01 respectively).
// This removes the duplicate retry loop that previously lived in both packages.
func Retry(ctx context.Context, isRetryable func(error) bool, op func() error) error {
	policy := backoff.WithContext(
		backoff.WithMaxRetries(backoff.NewExponentialBackOff(), 4),
		ctx,
	)

	return backoff.Retry(func() error {
		err := op()
		if err == nil {
			return nil
		}
		if isRetryable(err) {
			return err // retryable — backoff.Retry will sleep and try again
		}
		return backoff.Permanent(err) // non-retryable — stop immediately
	}, policy)
}
