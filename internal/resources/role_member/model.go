// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package role_member

import "github.com/hashicorp/terraform-plugin-framework/types"

// roleMemberModel maps the Terraform state for azsqlaccess_database_role_member
// to a typed Go struct. tfsdk tags must exactly match the attribute names in Schema.
type roleMemberModel struct {
	ID       types.String `tfsdk:"id"`
	Server   types.String `tfsdk:"server"`
	Database types.String `tfsdk:"database"`
	Role     types.String `tfsdk:"role"`
	Member   types.String `tfsdk:"member"`
}
