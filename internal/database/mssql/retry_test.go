// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package mssql

import (
	"errors"
	"fmt"
	"testing"

	mssqldb "github.com/microsoft/go-mssqldb"
)

func TestIsDeadlock(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"non-mssql error", errors.New("some other error"), false},
		{"mssql error 1205 (deadlock)", mssqldb.Error{Number: 1205}, true},
		{"mssql error 999", mssqldb.Error{Number: 999}, false},
		{"mssql error 0", mssqldb.Error{Number: 0}, false},
		{"wrapped 1205", fmt.Errorf("ddl failed: %w", mssqldb.Error{Number: 1205}), true},
		{"doubly wrapped 1205", fmt.Errorf("a: %w", fmt.Errorf("b: %w", mssqldb.Error{Number: 1205})), true},
		{"wrapped non-1205", fmt.Errorf("ddl failed: %w", mssqldb.Error{Number: 1234}), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isDeadlock(tc.err); got != tc.want {
				t.Fatalf("isDeadlock(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
