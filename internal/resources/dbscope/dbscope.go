// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

// Package dbscope plans a change of a resource's database attribute.
package dbscope

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/mews/terraform-provider-azsqlaccess/internal/database"
)

// PlanDatabaseChange is called once a resource's database differs from state. On an
// engine whose principals are server-scoped the change only moves the connection, so it
// updates in place and the computed id is recomputed at apply; otherwise, or when the
// provider is not configured yet, the resource is replaced.
func PlanDatabaseChange(ctx context.Context, factory database.ConnectorFactory, resp *resource.ModifyPlanResponse) {
	if factory == nil || !factory.ServerScopedPrincipals() {
		resp.RequiresReplace = append(resp.RequiresReplace, path.Root("database"))
		return
	}
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("id"), types.StringUnknown())...)
}
