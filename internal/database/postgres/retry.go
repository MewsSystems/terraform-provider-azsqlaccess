// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package postgres

import (
	"context"
	"errors"

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
