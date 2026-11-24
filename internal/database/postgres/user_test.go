// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package postgres

import (
	"slices"
	"testing"
)

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

func TestSplitSearchPath(t *testing.T) {
	cases := []struct {
		in   string
		want []string // nil: malformed
	}{
		{"reporting", []string{"reporting"}},
		{`"My Schema"`, []string{"My Schema"}},
		{`"a""b"`, []string{`a"b`}},
		{`""""`, []string{`"`}},
		{`"a,b"`, []string{"a,b"}},
		{`"$user", public`, []string{"$user", "public"}},
		{`"My Schema", "Other Schema"`, []string{"My Schema", "Other Schema"}},
		{" app ,\tpublic ", []string{"app", "public"}},
		{"", nil},
		{`""`, nil},
		{`"unterminated`, nil},
		{`"a"b`, nil},
		{"a,,b", nil},
		{"a,", nil},
		{",a", nil},
	}
	for _, tc := range cases {
		got, ok := splitSearchPath(tc.in)
		if tc.want == nil {
			if ok {
				t.Errorf("splitSearchPath(%q) = %q, want malformed", tc.in, got)
			}
			continue
		}
		if !ok || !slices.Equal(got, tc.want) {
			t.Errorf("splitSearchPath(%q) = %q, %v; want %q", tc.in, got, ok, tc.want)
		}
	}
}

// Whatever schema name Terraform writes, reading it back yields the same name.
func FuzzSplitSearchPathRoundTrip(f *testing.F) {
	for _, s := range []string{"reporting", "My Schema", `a"b`, `"`, "a, b", "$user", " x "} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, name string) {
		if name == "" {
			t.Skip("PostgreSQL rejects zero-length identifiers")
		}
		got, ok := splitSearchPath(quoteIdentifier(name))
		if !ok || len(got) != 1 || got[0] != name {
			t.Fatalf("round trip of %q gave %q, %v", name, got, ok)
		}
	})
}
