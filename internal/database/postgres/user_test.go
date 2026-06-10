// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package postgres

import "testing"

func TestQuoteIdentifier(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"abc", `"abc"`},
		{"", `""`},
		{"db.reader", `"db.reader"`},
		{"juan.perez@milanesa.com", `"juan.perez@milanesa.com"`},
		{"with space", `"with space"`},
		// Embedded " must double:
		{`a"b`, `"a""b"`},
		{`"`, `""""`},
		{`""`, `""""""`},
		// Brackets, semicolons, etc. are not special in PG identifiers:
		{"a]b", `"a]b"`},
		{"a;b", `"a;b"`},
	}

	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := quoteIdentifier(tc.in); got != tc.want {
				t.Fatalf("quoteIdentifier(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
