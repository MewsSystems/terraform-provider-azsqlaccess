// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package dbscope

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/mews/terraform-provider-azsqlaccess/internal/database"
)

type fakeFactory struct{ serverScoped bool }

func (f fakeFactory) GetConnector(string, string) (database.DatabaseConnector, error) {
	return nil, nil
}
func (f fakeFactory) ServerScopedPrincipals() bool { return f.serverScoped }

func response(t *testing.T) *resource.ModifyPlanResponse {
	t.Helper()
	s := schema.Schema{Attributes: map[string]schema.Attribute{
		"id":       schema.StringAttribute{Computed: true},
		"database": schema.StringAttribute{Required: true},
	}}
	objType := s.Type().TerraformType(context.Background()).(tftypes.Object)
	raw := tftypes.NewValue(objType, map[string]tftypes.Value{
		"id":       tftypes.NewValue(tftypes.String, "myserver.postgres.database.azure.com/mydb/x"),
		"database": tftypes.NewValue(tftypes.String, "postgres"),
	})
	return &resource.ModifyPlanResponse{Plan: tfsdk.Plan{Schema: s, Raw: raw}}
}

func TestPlanDatabaseChange_ServerScoped_InPlace(t *testing.T) {
	resp := response(t)
	PlanDatabaseChange(context.Background(), fakeFactory{serverScoped: true}, resp)
	if len(resp.RequiresReplace) != 0 {
		t.Fatalf("RequiresReplace = %v, want none", resp.RequiresReplace)
	}
	var id types.String
	resp.Plan.GetAttribute(context.Background(), path.Root("id"), &id)
	if !id.IsUnknown() {
		t.Fatalf("id = %v, want unknown so apply can set the new one", id)
	}
}

func TestPlanDatabaseChange_DatabaseScoped_RequiresReplace(t *testing.T) {
	resp := response(t)
	PlanDatabaseChange(context.Background(), fakeFactory{serverScoped: false}, resp)
	if !resp.RequiresReplace.Contains(path.Root("database")) {
		t.Fatalf("RequiresReplace = %v, want database", resp.RequiresReplace)
	}
}

func TestPlanDatabaseChange_NoFactory_RequiresReplace(t *testing.T) {
	resp := response(t)
	PlanDatabaseChange(context.Background(), nil, resp)
	if !resp.RequiresReplace.Contains(path.Root("database")) {
		t.Fatalf("RequiresReplace = %v, want database", resp.RequiresReplace)
	}
}
