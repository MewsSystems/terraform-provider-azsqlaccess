// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	pschema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/mews/terraform-provider-azsqlaccess/internal/database"
)

// ---------- helpers ---------------------------------------------------------

func providerSchema(t *testing.T) pschema.Schema {
	t.Helper()
	p := &AzsqlaccessProvider{}
	resp := &provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema build failed: %v", resp.Diagnostics)
	}
	return resp.Schema
}

// schemaObjectType returns the schema's tftypes.Object representation, failing
// the test if the schema's top-level type isn't an Object (which would indicate
// a framework regression).
func schemaObjectType(t *testing.T, ctx context.Context, schema pschema.Schema) tftypes.Object {
	t.Helper()
	objType, ok := schema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatalf("expected tftypes.Object schema type, got %T", schema.Type().TerraformType(ctx))
	}
	return objType
}

// ---------- pure tests ------------------------------------------------------

func TestProvider_Metadata(t *testing.T) {
	p := New("test-version")()
	resp := &provider.MetadataResponse{}
	p.Metadata(context.Background(), provider.MetadataRequest{}, resp)
	if resp.TypeName != "azsqlaccess" {
		t.Fatalf("TypeName = %q, want azsqlaccess", resp.TypeName)
	}
	if resp.Version != "test-version" {
		t.Fatalf("Version = %q, want test-version", resp.Version)
	}
}

func TestProvider_Schema_HasRequiredAttributes(t *testing.T) {
	s := providerSchema(t)
	want := []string{"engine", "tenant_id", "client_id", "client_secret"}
	for _, n := range want {
		if _, ok := s.Attributes[n]; !ok {
			t.Errorf("schema missing attribute %q", n)
		}
	}
	if s.MarkdownDescription == "" {
		t.Error("schema MarkdownDescription is empty")
	}
}

func TestProvider_Resources(t *testing.T) {
	p := &AzsqlaccessProvider{}
	got := p.Resources(context.Background())
	if len(got) != 2 {
		t.Fatalf("Resources() returned %d, want 2", len(got))
	}
	for i, fn := range got {
		if fn() == nil {
			t.Errorf("Resources[%d] constructor returned nil", i)
		}
	}
}

func TestProvider_DataSources(t *testing.T) {
	p := &AzsqlaccessProvider{}
	if got := p.DataSources(context.Background()); got != nil {
		t.Fatalf("DataSources() = %v, want nil", got)
	}
}

func TestProvider_New(t *testing.T) {
	fn := New("v0.1.0")
	if fn == nil {
		t.Fatal("New returned nil function")
	}
	p := fn()
	if p == nil {
		t.Fatal("New()() returned nil")
	}
	if _, ok := p.(*AzsqlaccessProvider); !ok {
		t.Fatalf("New produced %T, want *AzsqlaccessProvider", p)
	}
}

// ---------- Configure -------------------------------------------------------

func TestProvider_Configure_MSSQLEngine(t *testing.T) {
	ctx := context.Background()
	p := &AzsqlaccessProvider{}

	// Manually craft the config so engine is "mssql"; the credential fields
	// stay null so the default-credential branch is taken.
	objType := schemaObjectType(t, ctx, providerSchema(t))
	values := map[string]tftypes.Value{}
	for name, at := range objType.AttributeTypes {
		values[name] = tftypes.NewValue(at, nil)
	}
	values["engine"] = tftypes.NewValue(tftypes.String, "mssql")
	cfg := tfsdk.Config{Schema: providerSchema(t), Raw: tftypes.NewValue(objType, values)}

	resp := &provider.ConfigureResponse{}
	p.Configure(ctx, provider.ConfigureRequest{Config: cfg}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected: %v", resp.Diagnostics)
	}
	if _, ok := resp.ResourceData.(database.ConnectorFactory); !ok {
		t.Fatalf("ResourceData should implement ConnectorFactory, got %T", resp.ResourceData)
	}
}

func TestProvider_Configure_MSSQLEngine_WithServicePrincipal(t *testing.T) {
	ctx := context.Background()
	p := &AzsqlaccessProvider{}
	objType := schemaObjectType(t, ctx, providerSchema(t))
	cfg := tfsdk.Config{
		Schema: providerSchema(t),
		Raw: tftypes.NewValue(objType, map[string]tftypes.Value{
			"engine":        tftypes.NewValue(tftypes.String, "mssql"),
			"tenant_id":     tftypes.NewValue(tftypes.String, "00000000-0000-0000-0000-000000000000"),
			"client_id":     tftypes.NewValue(tftypes.String, "00000000-0000-0000-0000-000000000000"),
			"client_secret": tftypes.NewValue(tftypes.String, "secret"),
		}),
	}
	resp := &provider.ConfigureResponse{}
	p.Configure(ctx, provider.ConfigureRequest{Config: cfg}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected: %v", resp.Diagnostics)
	}
}

func TestProvider_Configure_PostgresEngine(t *testing.T) {
	ctx := context.Background()
	p := &AzsqlaccessProvider{}
	objType := schemaObjectType(t, ctx, providerSchema(t))
	cfg := tfsdk.Config{
		Schema: providerSchema(t),
		Raw: tftypes.NewValue(objType, map[string]tftypes.Value{
			"engine":        tftypes.NewValue(tftypes.String, "postgres"),
			"tenant_id":     tftypes.NewValue(tftypes.String, "00000000-0000-0000-0000-000000000000"),
			"client_id":     tftypes.NewValue(tftypes.String, "00000000-0000-0000-0000-000000000000"),
			"client_secret": tftypes.NewValue(tftypes.String, "secret"),
		}),
	}
	resp := &provider.ConfigureResponse{}
	p.Configure(ctx, provider.ConfigureRequest{Config: cfg}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected: %v", resp.Diagnostics)
	}
	if _, ok := resp.ResourceData.(database.ConnectorFactory); !ok {
		t.Fatalf("ResourceData should be a ConnectorFactory, got %T", resp.ResourceData)
	}
}

func TestProvider_Configure_UnsupportedEngine(t *testing.T) {
	ctx := context.Background()
	p := &AzsqlaccessProvider{}
	objType := schemaObjectType(t, ctx, providerSchema(t))
	cfg := tfsdk.Config{
		Schema: providerSchema(t),
		Raw: tftypes.NewValue(objType, map[string]tftypes.Value{
			"engine":        tftypes.NewValue(tftypes.String, "oracle"),
			"tenant_id":     tftypes.NewValue(tftypes.String, nil),
			"client_id":     tftypes.NewValue(tftypes.String, nil),
			"client_secret": tftypes.NewValue(tftypes.String, nil),
		}),
	}
	resp := &provider.ConfigureResponse{}
	p.Configure(ctx, provider.ConfigureRequest{Config: cfg}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatalf("unsupported engine should produce error")
	}
	joined := ""
	for _, d := range resp.Diagnostics.Errors() {
		joined += d.Summary() + " " + d.Detail()
	}
	if !strings.Contains(joined, "engine must be") {
		t.Fatalf("unexpected error message: %s", joined)
	}
}
