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
