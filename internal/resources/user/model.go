// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package user

import "github.com/hashicorp/terraform-plugin-framework/types"

// userModel maps the Terraform state for azsqlaccess_user to a typed Go struct.
// tfsdk tags must exactly match the attribute names declared in the Schema.
type userModel struct {
	ID            types.String `tfsdk:"id"`
	Server        types.String `tfsdk:"server"`
	Database      types.String `tfsdk:"database"`
	Type          types.String `tfsdk:"type"`
	Name          types.String `tfsdk:"name"`
	ObjectID      types.String `tfsdk:"object_id"`
	DefaultSchema types.String `tfsdk:"default_schema"`
	PrincipalID   types.Int64  `tfsdk:"principal_id"`
}
