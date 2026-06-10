// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package user

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

// emptyState builds a fresh tfsdk.State whose Raw value is an Object with all
// schema attributes set to null. SetAttribute requires this — a fully-null
// Raw (i.e. tftypes.NewValue(t, nil)) makes attribute writes fail because
// there is no Object to write into.
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

func userSchema(t *testing.T) rschema.Schema {
	t.Helper()
	r := &UserResource{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema build failed: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func TestUserResource_Metadata(t *testing.T) {
	r := &UserResource{}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "azsqlaccess"}, resp)
	if resp.TypeName != "azsqlaccess_user" {
		t.Fatalf("TypeName = %q, want azsqlaccess_user", resp.TypeName)
	}
}

func TestUserResource_NewResource(t *testing.T) {
	got := NewResource()
	if got == nil {
		t.Fatalf("NewResource returned nil")
	}
	if _, ok := got.(*UserResource); !ok {
		t.Fatalf("NewResource returned %T, want *UserResource", got)
	}
}

func TestUserResource_Schema_HasRequiredAttributes(t *testing.T) {
	s := userSchema(t)
	want := []string{"id", "server", "database", "type", "name", "object_id", "default_schema", "principal_id"}
	for _, name := range want {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("schema missing attribute %q", name)
		}
	}
	if s.MarkdownDescription == "" {
		t.Error("schema MarkdownDescription is empty")
	}
}

// ----- ImportState ---------------------------------------------------------

func TestUserResource_ImportState_UserSuccess(t *testing.T) {
	ctx := context.Background()
	r := &UserResource{}
	resp := &resource.ImportStateResponse{State: emptyState(ctx, t, userSchema(t))}
	r.ImportState(ctx, resource.ImportStateRequest{
		ID: "myserver.database.windows.net/mydb/user/juan.perez@milanesa.com",
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	var got string
	resp.State.GetAttribute(ctx, path.Root("id"), &got)
	if got != "myserver.database.windows.net/mydb/user/juan.perez@milanesa.com" {
		t.Errorf("id = %q", got)
	}
	resp.State.GetAttribute(ctx, path.Root("type"), &got)
	if got != "user" {
		t.Errorf("type = %q", got)
	}
	resp.State.GetAttribute(ctx, path.Root("name"), &got)
	if got != "juan.perez@milanesa.com" {
		t.Errorf("name = %q", got)
	}
	resp.State.GetAttribute(ctx, path.Root("server"), &got)
	if got != "myserver.database.windows.net" {
		t.Errorf("server = %q", got)
	}
	resp.State.GetAttribute(ctx, path.Root("database"), &got)
	if got != "mydb" {
		t.Errorf("database = %q", got)
	}
}

func TestUserResource_ImportState_GroupSuccess(t *testing.T) {
	ctx := context.Background()
	r := &UserResource{}
	resp := &resource.ImportStateResponse{State: emptyState(ctx, t, userSchema(t))}
	r.ImportState(ctx, resource.ImportStateRequest{
		ID: "myserver.database.windows.net/mydb/group/db.reader/00000000-0000-0000-0000-000000000000",
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	var got string
	resp.State.GetAttribute(ctx, path.Root("type"), &got)
	if got != "group" {
		t.Errorf("type = %q", got)
	}
	resp.State.GetAttribute(ctx, path.Root("object_id"), &got)
	if got != "00000000-0000-0000-0000-000000000000" {
		t.Errorf("object_id = %q", got)
	}
}

func TestUserResource_ImportState_ServicePrincipalSuccess(t *testing.T) {
	ctx := context.Background()
	r := &UserResource{}
	resp := &resource.ImportStateResponse{State: emptyState(ctx, t, userSchema(t))}
	r.ImportState(ctx, resource.ImportStateRequest{
		ID: "myserver.database.windows.net/mydb/service_principal/myapp-identity/00000000-0000-0000-0000-000000000000",
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	var got string
	resp.State.GetAttribute(ctx, path.Root("type"), &got)
	if got != "service_principal" {
		t.Errorf("type = %q", got)
	}
	resp.State.GetAttribute(ctx, path.Root("name"), &got)
	if got != "myapp-identity" {
		t.Errorf("name = %q", got)
	}
}

func TestUserResource_ImportState_Errors(t *testing.T) {
	cases := []struct {
		name        string
		id          string
		wantContain string
	}{
		{"too few segments", "a/b", "expected format"},
		{"empty server", "/mydb/user/juan.perez@milanesa.com", "expected format"},
		{"empty database", "myserver.database.windows.net//user/juan.perez@milanesa.com", "expected format"},
		{"empty type", "myserver.database.windows.net/mydb//juan.perez@milanesa.com", "expected format"},
		{"empty name", "myserver.database.windows.net/mydb/user/", "expected format"},
		{"server not fqdn", "myserver/mydb/user/juan.perez@milanesa.com", "fully-qualified hostname"},
		{"user with object_id", "myserver.database.windows.net/mydb/user/juan.perez@milanesa.com/00000000-0000-0000-0000-000000000000", "does not use object_id"},
		{"group missing object_id", "myserver.database.windows.net/mydb/group/db.reader", "requires object_id"},
		{"sp missing object_id", "myserver.database.windows.net/mydb/service_principal/myapp-identity", "requires object_id"},
		{"group empty object_id", "myserver.database.windows.net/mydb/group/db.reader/", "requires object_id"},
		{"unknown type", "myserver.database.windows.net/mydb/admin/foo", "must be one of"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			r := &UserResource{}
			resp := &resource.ImportStateResponse{State: emptyState(ctx, t, userSchema(t))}
			r.ImportState(ctx, resource.ImportStateRequest{ID: tc.id}, resp)
			if !resp.Diagnostics.HasError() {
				t.Fatalf("expected error for %q", tc.id)
			}
			joined := ""
			for _, d := range resp.Diagnostics.Errors() {
				joined += d.Summary() + " " + d.Detail() + "\n"
			}
			if !strings.Contains(joined, tc.wantContain) {
				t.Fatalf("error message missing %q\ngot: %s", tc.wantContain, joined)
			}
		})
	}
}

// ----- ValidateConfig ------------------------------------------------------

// configFromModel converts a populated userModel into a tfsdk.Config that
// ValidateConfig can read.
func configFromModel(ctx context.Context, t *testing.T, schema rschema.Schema, m userModel) tfsdk.Config {
	t.Helper()
	state := emptyState(ctx, t, schema)
	diags := state.Set(ctx, &m)
	if diags.HasError() {
		t.Fatalf("State.Set failed: %v", diags)
	}
	return tfsdk.Config{Schema: schema, Raw: state.Raw}
}

func TestUserResource_ValidateConfig(t *testing.T) {
	ctx := context.Background()
	schema := userSchema(t)

	cases := []struct {
		name      string
		model     userModel
		wantError bool
		want      string
	}{
		{
			name: "user without object_id ok",
			model: userModel{
				Type: stringValue("user"),
				Name: stringValue("juan.perez@milanesa.com"),
			},
			wantError: false,
		},
		{
			name: "user with object_id rejected",
			model: userModel{
				Type:     stringValue("user"),
				Name:     stringValue("juan.perez@milanesa.com"),
				ObjectID: stringValue("00000000-0000-0000-0000-000000000000"),
			},
			wantError: true,
			want:      "must not be set",
		},
		{
			name: "group with object_id ok",
			model: userModel{
				Type:     stringValue("group"),
				Name:     stringValue("db.reader"),
				ObjectID: stringValue("00000000-0000-0000-0000-000000000000"),
			},
			wantError: false,
		},
		{
			name: "group without object_id rejected",
			model: userModel{
				Type: stringValue("group"),
				Name: stringValue("db.reader"),
			},
			wantError: true,
			want:      "is required",
		},
		{
			name: "service_principal with object_id ok",
			model: userModel{
				Type:     stringValue("service_principal"),
				Name:     stringValue("myapp-identity"),
				ObjectID: stringValue("00000000-0000-0000-0000-000000000000"),
			},
			wantError: false,
		},
		{
			name: "service_principal without object_id rejected",
			model: userModel{
				Type: stringValue("service_principal"),
				Name: stringValue("myapp-identity"),
			},
			wantError: true,
			want:      "is required",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &UserResource{}
			cfg := configFromModel(ctx, t, schema, tc.model)
			req := resource.ValidateConfigRequest{Config: cfg}
			resp := &resource.ValidateConfigResponse{}
			r.ValidateConfig(ctx, req, resp)
			gotErr := resp.Diagnostics.HasError()
			if gotErr != tc.wantError {
				t.Fatalf("HasError = %v, want %v; diags=%v", gotErr, tc.wantError, resp.Diagnostics)
			}
			if tc.want != "" {
				joined := ""
				for _, d := range resp.Diagnostics.Errors() {
					joined += d.Summary() + " " + d.Detail() + "\n"
				}
				if !strings.Contains(joined, tc.want) {
					t.Fatalf("error message missing %q\ngot: %s", tc.want, joined)
				}
			}
		})
	}
}

func TestUserResource_ValidateConfig_UnknownType_Skipped(t *testing.T) {
	ctx := context.Background()
	schema := userSchema(t)

	// Type is unknown — the framework will re-call once resolved; we should not
	// emit an error in this state.
	objType, ok := schema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatalf("expected tftypes.Object schema type, got %T", schema.Type().TerraformType(ctx))
	}
	values := map[string]tftypes.Value{}
	for name, attrType := range objType.AttributeTypes {
		values[name] = tftypes.NewValue(attrType, nil)
	}
	values["type"] = tftypes.NewValue(tftypes.String, tftypes.UnknownValue)
	values["name"] = tftypes.NewValue(tftypes.String, "juan.perez@milanesa.com")
	cfg := tfsdk.Config{
		Schema: schema,
		Raw:    tftypes.NewValue(objType, values),
	}

	r := &UserResource{}
	resp := &resource.ValidateConfigResponse{}
	r.ValidateConfig(ctx, resource.ValidateConfigRequest{Config: cfg}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ValidateConfig with unknown type must not error; got %v", resp.Diagnostics)
	}
}
