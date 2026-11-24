// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package postgres

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsDeadlock(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"non-pg error", errors.New("plain error"), false},
		{"pg deadlock 40P01", &pgconn.PgError{Code: "40P01"}, true},
		{"pg other code", &pgconn.PgError{Code: "42883"}, false},
		{"pg empty code", &pgconn.PgError{Code: ""}, false},
		{"wrapped 40P01", fmt.Errorf("exec: %w", &pgconn.PgError{Code: "40P01"}), true},
		{"doubly wrapped 40P01", fmt.Errorf("a: %w", fmt.Errorf("b: %w", &pgconn.PgError{Code: "40P01"})), true},
		{"wrapped non-deadlock", fmt.Errorf("exec: %w", &pgconn.PgError{Code: "23505"}), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isDeadlock(tc.err); got != tc.want {
				t.Fatalf("isDeadlock(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// Message as Azure Database for PostgreSQL returns it for a principal Entra has
// not replicated yet, with this repo's placeholders substituted.
const principalNotFoundMsg = `Cannot validate Microsoft Entra ID user "myapp-identity" with OID ` +
	`"00000000-0000-0000-0000-000000000000" because the OID isn't found in the tenant. ` +
	`Make sure it's correct and retry.`

func TestIsPrincipalNotFound(t *testing.T) {
	notFound := &pgconn.PgError{Code: "0LP01", Message: principalNotFoundMsg}
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"non-pg error with the same text", errors.New(principalNotFoundMsg), false},
		{"0LP01 OID not found", notFound, true},
		{"wrapped 0LP01 OID not found", fmt.Errorf("pgaadauth_create_principal_with_oid: %w", notFound), true},
		{"0LP01 other message", &pgconn.PgError{Code: "0LP01", Message: "role name does not match"}, false},
		{"other code same message", &pgconn.PgError{Code: "42704", Message: principalNotFoundMsg}, false},
		{"deadlock", &pgconn.PgError{Code: "40P01"}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isPrincipalNotFound(tc.err); got != tc.want {
				t.Fatalf("isPrincipalNotFound(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
