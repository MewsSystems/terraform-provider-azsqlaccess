// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/jackc/pgx/v5/pgconn"
	database "github.com/mews/terraform-provider-azsqlaccess/internal/database"
)

// isDeadlock reports whether err is a PostgreSQL deadlock error (SQLSTATE 40P01).
// PostgreSQL raises this when two transactions deadlock; the failing transaction
// is safe to retry.
func isDeadlock(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "40P01"
}

// withRetry executes op with exponential backoff, retrying only on deadlock (40P01).
// The retry loop is shared with the mssql package via database.Retry.
func withRetry(ctx context.Context, op func() error) error {
	return database.Retry(ctx, isDeadlock, op)
}

// isPrincipalNotFound reports whether err is the server failing to find a
// principal's object ID in Entra (SQLSTATE 0LP01, "... because the OID isn't
// found in the tenant"). Entra replicates a new principal, such as a managed
// identity created in the same apply, asynchronously, so the lookup can miss
// it for a while. The role was not created, so the call is safe to repeat.
// 0LP01 covers other pgaadauth validation failures too, hence the message check.
func isPrincipalNotFound(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "0LP01" &&
		strings.Contains(pgErr.Message, "isn't found in the tenant")
}

// newPrincipalPropagationBackOff bounds the wait for Entra replication at
// three minutes, so an object ID that is simply wrong still fails the apply.
// A variable so tests can replace the waits.
var newPrincipalPropagationBackOff = func() backoff.BackOff {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = 2 * time.Second
	b.MaxInterval = 15 * time.Second
	b.MaxElapsedTime = 3 * time.Minute
	return b
}

// withPrincipalPropagationRetry executes op, retrying only while the server
// cannot find the principal in Entra.
func withPrincipalPropagationRetry(ctx context.Context, op func() error) error {
	return backoff.Retry(func() error {
		err := op()
		if err == nil || isPrincipalNotFound(err) {
			return err
		}
		return backoff.Permanent(err)
	}, backoff.WithContext(newPrincipalPropagationBackOff(), ctx))
}
