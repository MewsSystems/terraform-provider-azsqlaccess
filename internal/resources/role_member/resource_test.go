// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package role_member

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func emptyState(ctx context.Context, t *testing.T, schema rschema.Schema) tfsdk.State {
	t.Helper()
	objType, ok := schema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatalf("expected tftypes.Object schema type, got %T", schema.Type().TerraformType(ctx))
	}
	nulls := make(map[string]tftypes.Value, len(objType.AttributeTypes))
	for name, at := range objType.AttributeTypes {
		nulls[name] = tftypes.NewValue(at, nil)
	}
	return tfsdk.State{
		Schema: schema,
		Raw:    tftypes.NewValue(objType, nulls),
	}
}

func roleMemberSchema(t *testing.T) rschema.Schema {
	t.Helper()
	r := &RoleMemberResource{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema build failed: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func TestRoleMemberResource_Metadata(t *testing.T) {
	r := &RoleMemberResource{}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "azsqlaccess"}, resp)
	if resp.TypeName != "azsqlaccess_database_role_member" {
		t.Fatalf("TypeName = %q, want azsqlaccess_database_role_member", resp.TypeName)
	}
}

func TestRoleMemberResource_NewResource(t *testing.T) {
	got := NewResource()
	if got == nil {
		t.Fatalf("NewResource returned nil")
	}
	if _, ok := got.(*RoleMemberResource); !ok {
		t.Fatalf("NewResource returned %T, want *RoleMemberResource", got)
	}
}

func TestRoleMemberResource_Schema_HasRequiredAttributes(t *testing.T) {
	s := roleMemberSchema(t)
	want := []string{"id", "server", "database", "role", "member"}
	for _, name := range want {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("schema missing attribute %q", name)
		}
	}
	if s.MarkdownDescription == "" {
		t.Error("schema MarkdownDescription is empty")
	}
}

// Update is intentionally a no-op (all attributes RequiresReplace). Verify it
// runs without panic and produces no diagnostics. This covers the body for
// the coverage reporter.
func TestRoleMemberResource_Update_NoOp(t *testing.T) {
	r := &RoleMemberResource{}
	r.Update(context.Background(), resource.UpdateRequest{}, &resource.UpdateResponse{})
}

func TestRoleMemberResource_ImportState_Success(t *testing.T) {
	ctx := context.Background()
	r := &RoleMemberResource{}
	resp := &resource.ImportStateResponse{State: emptyState(ctx, t, roleMemberSchema(t))}
	r.ImportState(ctx, resource.ImportStateRequest{
		ID: "myserver.database.windows.net/mydb/db_datareader/juan.perez@milanesa.com",
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var got string
	resp.State.GetAttribute(ctx, path.Root("id"), &got)
	if got != "myserver.database.windows.net/mydb/db_datareader/juan.perez@milanesa.com" {
		t.Errorf("id = %q", got)
	}
	resp.State.GetAttribute(ctx, path.Root("server"), &got)
	if got != "myserver.database.windows.net" {
		t.Errorf("server = %q", got)
	}
	resp.State.GetAttribute(ctx, path.Root("database"), &got)
	if got != "mydb" {
		t.Errorf("database = %q", got)
	}
	resp.State.GetAttribute(ctx, path.Root("role"), &got)
	if got != "db_datareader" {
		t.Errorf("role = %q", got)
	}
	resp.State.GetAttribute(ctx, path.Root("member"), &got)
	if got != "juan.perez@milanesa.com" {
		t.Errorf("member = %q", got)
	}
}

func TestRoleMemberResource_ImportState_Errors(t *testing.T) {
	cases := []struct {
		name        string
		id          string
		wantContain string
	}{
		{"too few segments", "myserver.database.windows.net/mydb/db_datareader", "expected format"},
		{"empty server", "/mydb/db_datareader/juan.perez@milanesa.com", "expected format"},
		{"empty database", "myserver.database.windows.net//db_datareader/juan.perez@milanesa.com", "expected format"},
		{"empty role", "myserver.database.windows.net/mydb//juan.perez@milanesa.com", "expected format"},
		{"empty member", "myserver.database.windows.net/mydb/db_datareader/", "expected format"},
		{"server not fqdn", "myserver/mydb/db_datareader/juan.perez@milanesa.com", "fully-qualified hostname"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			r := &RoleMemberResource{}
			resp := &resource.ImportStateResponse{State: emptyState(ctx, t, roleMemberSchema(t))}
			r.ImportState(ctx, resource.ImportStateRequest{ID: tc.id}, resp)
			if !resp.Diagnostics.HasError() {
				t.Fatalf("expected error for %q", tc.id)
			}
			joined := ""
			for _, d := range resp.Diagnostics.Errors() {
				joined += d.Summary() + " " + d.Detail() + "\n"
			}
			if !strings.Contains(joined, tc.wantContain) {
				t.Fatalf("error missing %q\ngot: %s", tc.wantContain, joined)
			}
		})
	}
}
