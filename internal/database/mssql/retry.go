// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package mssql

import (
	"context"
	"errors"

	database "github.com/mews/terraform-provider-azsqlaccess/internal/database"
	mssqldb "github.com/microsoft/go-mssqldb"
)

// isDeadlock reports whether err is MSSQL error 1205 — the deadlock victim error.
//
// Azure SQL raises 1205 when two concurrent transactions deadlock and this
// transaction is chosen as the victim. The SQL Server docs explicitly say
// "Rerun the transaction", making it safe to retry transparently.
func isDeadlock(err error) bool {
	var mssqlErr mssqldb.Error
	return errors.As(err, &mssqlErr) && mssqlErr.Number == 1205
}

// withRetry executes op with exponential backoff, retrying only on deadlock (1205).
// The retry loop is shared with the postgres package via database.Retry.
func withRetry(ctx context.Context, op func() error) error {
	return database.Retry(ctx, isDeadlock, op)
}
