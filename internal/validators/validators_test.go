// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package validators

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func newStringRequest(value types.String) validator.StringRequest {
	return validator.StringRequest{
		Path:        path.Root("attr"),
		ConfigValue: value,
	}
}

func TestStringOneOf_Description(t *testing.T) {
	v := StringOneOf("a", "b", "c")
	desc := v.Description(context.Background())
	if !strings.Contains(desc, "a, b, c") {
		t.Fatalf("Description missing values: %q", desc)
	}
	if md := v.MarkdownDescription(context.Background()); md != desc {
		t.Fatalf("MarkdownDescription should match Description, got %q vs %q", md, desc)
	}
}

func TestStringOneOf_Validate(t *testing.T) {
	v := StringOneOf("mssql", "postgres")

	cases := []struct {
		name    string
		input   types.String
		wantErr bool
	}{
		{"valid first", types.StringValue("mssql"), false},
		{"valid second", types.StringValue("postgres"), false},
		{"invalid", types.StringValue("oracle"), true},
		{"empty", types.StringValue(""), true},
		{"null skipped", types.StringNull(), false},
		{"unknown skipped", types.StringUnknown(), false},
		{"case sensitive", types.StringValue("MSSQL"), true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := newStringRequest(tc.input)
			resp := &validator.StringResponse{Diagnostics: diag.Diagnostics{}}
			v.ValidateString(context.Background(), req, resp)
			if got := resp.Diagnostics.HasError(); got != tc.wantErr {
				t.Fatalf("HasError = %v want %v; diags=%v", got, tc.wantErr, resp.Diagnostics)
			}
		})
	}
}

func TestStringUUID_Description(t *testing.T) {
	v := StringUUID()
	desc := v.Description(context.Background())
	if !strings.Contains(desc, "UUID") {
		t.Fatalf("Description should mention UUID: %q", desc)
	}
	if md := v.MarkdownDescription(context.Background()); md != desc {
		t.Fatalf("MarkdownDescription should match Description")
	}
}

func TestStringUUID_Validate(t *testing.T) {
	v := StringUUID()

	cases := []struct {
		name    string
		input   types.String
		wantErr bool
	}{
		{"canonical", types.StringValue("00000000-0000-0000-0000-000000000000"), false},
		{"hex with hyphens", types.StringValue("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"), false},
		{"uppercase accepted", types.StringValue("AAAAAAAA-BBBB-CCCC-DDDD-EEEEEEEEEEEE"), false},
		{"no hyphens", types.StringValue("aaaaaaaabbbbccccddddeeeeeeeeeeee"), false},
		{"too short", types.StringValue("not-a-uuid"), true},
		{"obvious garbage", types.StringValue("zzzzzzzz-zzzz-zzzz-zzzz-zzzzzzzzzzzz"), true},
		{"empty", types.StringValue(""), true},
		{"null skipped", types.StringNull(), false},
		{"unknown skipped", types.StringUnknown(), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := newStringRequest(tc.input)
			resp := &validator.StringResponse{Diagnostics: diag.Diagnostics{}}
			v.ValidateString(context.Background(), req, resp)
			if got := resp.Diagnostics.HasError(); got != tc.wantErr {
				t.Fatalf("HasError = %v want %v; diags=%v", got, tc.wantErr, resp.Diagnostics)
			}
		})
	}
}
