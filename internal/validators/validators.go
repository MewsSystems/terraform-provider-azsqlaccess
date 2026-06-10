// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

// Package validators provides reusable terraform-plugin-framework string
// validators without requiring the external terraform-plugin-framework-validators
// module. Implementing the schema/validator.String interface directly keeps
// the dependency surface minimal.
package validators

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// StringOneOf returns a validator that rejects any value not in the allowed set.
// The check is case-sensitive. Null and unknown values are skipped (the Required
// constraint handles presence separately).
//
// Usage:
//
//	Validators: []validator.String{validators.StringOneOf("mssql", "postgres")},
func StringOneOf(values ...string) validator.String {
	return stringOneOf{valid: values}
}

type stringOneOf struct {
	valid []string
}

func (v stringOneOf) Description(_ context.Context) string {
	return fmt.Sprintf("value must be one of: %s", strings.Join(v.valid, ", "))
}

func (v stringOneOf) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v stringOneOf) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	val := req.ConfigValue.ValueString()
	for _, allowed := range v.valid {
		if val == allowed {
			return
		}
	}
	resp.Diagnostics.AddAttributeError(
		req.Path,
		"Invalid value",
		fmt.Sprintf("%q is not a recognised value; expected one of: %s", val, strings.Join(v.valid, ", ")),
	)
}

// StringUUID returns a validator that rejects any value that is not a valid UUID
// (RFC 4122, case-insensitive, with or without hyphens).
// Null and unknown values are skipped.
//
// Usage:
//
//	Validators: []validator.String{validators.StringUUID()},
func StringUUID() validator.String {
	return stringUUID{}
}

type stringUUID struct{}

func (v stringUUID) Description(_ context.Context) string {
	return "value must be a valid UUID (e.g. 00000000-0000-0000-0000-000000000000)"
}

func (v stringUUID) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v stringUUID) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	val := req.ConfigValue.ValueString()
	if _, err := uuid.Parse(val); err != nil {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid UUID",
			fmt.Sprintf("%q is not a valid UUID: %s", val, err),
		)
	}
}
