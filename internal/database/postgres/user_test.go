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

func TestSchemaFromSearchPath(t *testing.T) {
	cases := []struct{ stored, want string }{
		// One schema: decoded.
		{"reporting", "reporting"},
		{`"My Schema"`, "My Schema"},
		{`"a""b"`, `a"b`},
		{`""""`, `"`},
		{`"a, b"`, "a, b"},
		{" \tapp ", "app"},
		// Anything else: kept as stored.
		{"", ""},
		{`"$user", public`, `"$user", public`},
		{`"My Schema", "Other Schema"`, `"My Schema", "Other Schema"`},
		{`""`, `""`},
		{`"unterminated`, `"unterminated`},
		{`"a"b`, `"a"b`},
		{"a,", "a,"},
	}
	for _, tc := range cases {
		if got := schemaFromSearchPath(tc.stored); got != tc.want {
			t.Errorf("schemaFromSearchPath(%q) = %q, want %q", tc.stored, got, tc.want)
		}
	}
}

// Whatever schema name Terraform writes, reading it back yields the same name.
func FuzzSchemaFromSearchPathRoundTrip(f *testing.F) {
	for _, s := range []string{"reporting", "My Schema", `a"b`, `"`, "a, b", "$user", " x "} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, name string) {
		if name == "" {
			t.Skip("PostgreSQL rejects zero-length identifiers")
		}
		if got := schemaFromSearchPath(quoteIdentifier(name)); got != name {
			t.Fatalf("round trip of %q gave %q", name, got)
		}
	})
}
