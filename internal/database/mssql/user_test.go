// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package mssql

import (
	"errors"
	"strings"
	"testing"
)

func TestQuoteName(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"abc", "[abc]"},
		{"", "[]"},
		{"with space", "[with space]"},
		{"db_datareader", "[db_datareader]"},
		{"juan.perez@milanesa.com", "[juan.perez@milanesa.com]"},
		{"db.reader", "[db.reader]"},
		// Embedded ] must double:
		{"weird]name", "[weird]]name]"},
		{"]", "[]]]"},
		{"]]", "[]]]]]"},
		// Other special characters are left alone — only ] is special inside [].
		{`a"b`, `[a"b]`},
		{"a'b", "[a'b]"},
		{"a;b", "[a;b]"},
	}

	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := quoteName(tc.in); got != tc.want {
				t.Fatalf("quoteName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestTranslateCreateUserError(t *testing.T) {
	cases := []struct {
		name        string
		inputErr    error
		wantContain []string
	}{
		{
			name:     "begin-with-original",
			inputErr: errors.New("Principal name with object id 'X' must begin with the original principal name"),
			wantContain: []string{
				"name/object_id combination",
				"Entra display name",
				"UPN",
			},
		},
		{
			name:        "already-exists",
			inputErr:    errors.New("User or role 'foo' already exists in the current database."),
			wantContain: []string{"already exists", "import", "terraform import"},
		},
		{
			name:        "fallback",
			inputErr:    errors.New("some unexpected mssql failure"),
			wantContain: []string{"CREATE USER:", "some unexpected mssql failure"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := translateCreateUserError("juan.perez@milanesa.com", tc.inputErr)
			if got == nil {
				t.Fatalf("expected non-nil error")
			}
			if !errors.Is(got, tc.inputErr) {
				t.Fatalf("original error must be preserved via %%w; got %v", got)
			}
			msg := got.Error()
			for _, want := range tc.wantContain {
				if !strings.Contains(msg, want) {
					t.Fatalf("error message missing %q\ngot: %s", want, msg)
				}
			}
		})
	}
}
