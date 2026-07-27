// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package user

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/mews/terraform-provider-azsqlaccess/internal/database"
	"github.com/mews/terraform-provider-azsqlaccess/internal/validators"
)

// Compile-time assertions.
var _ resource.Resource = &UserResource{}
var _ resource.ResourceWithConfigure = &UserResource{}
var _ resource.ResourceWithImportState = &UserResource{}
var _ resource.ResourceWithValidateConfig = &UserResource{}

// UserResource manages a azsqlaccess_user resource.
type UserResource struct {
	factory database.ConnectorFactory
}

func NewResource() resource.Resource {
	return &UserResource{}
}

func (r *UserResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (r *UserResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Creates an Entra identity as a contained user inside a specific database. " +
			"Supports member users (`type = \"user\"`), security groups (`type = \"group\"`), " +
			"and service principals / managed identities (`type = \"service_principal\"`).",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"server": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Fully-qualified SQL server hostname (e.g. `myserver.database.windows.net`).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"database": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name of the database in which to create the user.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"type": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Entra principal type. Determines how the identity is resolved and which " +
					"SQL variant is used on each engine:\n\n" +
					"- `\"user\"` — member user account. `name` must be the UPN " +
					"(e.g. `\"juan.perez@milanesa.com\"`). No `object_id` needed — UPNs are globally unique.\n" +
					"- `\"group\"` — security or Microsoft 365 group. `name` must be the display name. " +
					"`object_id` is required because multiple groups can share the same display name.\n" +
					"- `\"service_principal\"` — app registration or managed identity. `name` must be the " +
					"display name (e.g. `\"myapp-identity\"`). `object_id` is required.\n\n" +
					"Changing this forces resource replacement.",
				Validators: []validator.String{
					validators.StringOneOf("user", "group", "service_principal"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Principal name for the contained database user:\n\n" +
					"- `type = \"user\"` → UPN, e.g. `\"juan.perez@milanesa.com\"`. Works on both engines.\n" +
					"- `type = \"group\"` → Entra display name, e.g. `\"db.reader\"`. " +
					"Works on both engines.\n" +
					"- `type = \"service_principal\"` → Azure resource display name, " +
					"e.g. `\"myapp-identity\"`. Works on both engines.\n\n" +
					"Changing this forces resource replacement.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"object_id": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "Entra Object (principal) ID of the group or service principal. " +
					"**Required for `type = \"group\"` and `type = \"service_principal\"`; " +
					"must not be set for `type = \"user\"`.**\n\n" +
					"For MSSQL this maps to `WITH OBJECT_ID = '...'` in the `CREATE USER` statement. " +
					"For PostgreSQL this maps to the second argument of `pgaadauth_create_principal_with_oid`. " +
					"Both engines use the same Entra Object (principal) ID — the value shown as " +
					"\"Object (principal) ID\" in the Azure portal.\n\n" +
					"Changing an already-set value forces resource replacement. " +
					"Setting it for the first time after `terraform import` does not — " +
					"the user already exists in the database and the value is simply recorded in state.",
				Validators: []validator.String{
					validators.StringUUID(),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"default_schema": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Default schema for the user. " +
					"For MSSQL this maps to `DEFAULT_SCHEMA` (defaults to `dbo` when unset). " +
					"For PostgreSQL this sets `search_path` on the role (defaults to `public` when unset).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"principal_id": schema.Int64Attribute{
				Computed: true,
				MarkdownDescription: "Internal identifier assigned by the database engine (computed). " +
					"Maps to `principal_id` in `sys.database_principals` for MSSQL and `pg_roles.oid` for PostgreSQL.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

// ValidateConfig enforces cross-attribute rules that cannot be expressed in
// individual attribute schemas:
//
//   - type = "user"              → object_id must NOT be set
//   - type = "group"             → object_id is required
//   - type = "service_principal" → object_id is required
func (r *UserResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config userModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// type may still be unknown during planning (e.g. computed from another resource).
	// Skip validation in that case — framework will call us again once it is known.
	if config.Type.IsUnknown() {
		return
	}

	switch config.Type.ValueString() {
	case "user":
		if !config.ObjectID.IsNull() && !config.ObjectID.IsUnknown() {
			resp.Diagnostics.AddAttributeError(
				path.Root("object_id"),
				"object_id must not be set for type = \"user\"",
				"UPNs are globally unique in Entra — object_id is redundant and not supported for user accounts. "+
					"Remove the object_id attribute.",
			)
		}
	case "group", "service_principal":
		if config.ObjectID.IsUnknown() {
			// object_id may still be unknown during planning (e.g. computed from a
			// data source or another resource) — skip validation here, framework
			// will call us again once it is known.
			return
		}
		if config.ObjectID.IsNull() {
			resp.Diagnostics.AddAttributeError(
				path.Root("object_id"),
				fmt.Sprintf("object_id is required for type = %q", config.Type.ValueString()),
				"Display names are not unique in Entra — multiple groups or service principals can share "+
					"the same name. Provide the Entra Object (principal) ID to ensure unambiguous resolution. "+
					"You can find it in the Azure portal under the identity's Overview blade.",
			)
		}
	}
}

// Configure receives the ConnectorFactory stored by the provider during Configure.
func (r *UserResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	factory, ok := req.ProviderData.(database.ConnectorFactory)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data type",
			fmt.Sprintf("expected database.ConnectorFactory, got %T — this is a provider bug", req.ProviderData),
		)
		return
	}
	r.factory = factory
}

func (r *UserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan userModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	conn, err := r.factory.GetConnector(plan.Server.ValueString(), plan.Database.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to get connector",
			fmt.Sprintf("server %q database %q: %s", plan.Server.ValueString(), plan.Database.ValueString(), err),
		)
		return
	}
	defer conn.Close()

	user := &database.User{
		Name:          plan.Name.ValueString(),
		Type:          plan.Type.ValueString(),
		DefaultSchema: plan.DefaultSchema.ValueString(),
		ObjectID:      plan.ObjectID.ValueString(),
	}

	if err := conn.CreateUser(ctx, user); err != nil {
		resp.Diagnostics.AddError(
			"Failed to create database user",
			fmt.Sprintf("user %q (type=%s) in database %q on %q: %s",
				plan.Name.ValueString(), plan.Type.ValueString(),
				plan.Database.ValueString(), plan.Server.ValueString(), err),
		)
		return
	}

	// ID format mirrors the import ID so that existing state is always importable:
	//   type=user              → server/database/user/name
	//   type=group/sp          → server/database/type/name/object_id
	var resourceID string
	if plan.Type.ValueString() == "user" {
		resourceID = plan.Server.ValueString() + "/" + plan.Database.ValueString() + "/" +
			plan.Type.ValueString() + "/" + plan.Name.ValueString()
	} else {
		resourceID = plan.Server.ValueString() + "/" + plan.Database.ValueString() + "/" +
			plan.Type.ValueString() + "/" + plan.Name.ValueString() + "/" + plan.ObjectID.ValueString()
	}
	plan.ID = types.StringValue(resourceID)
	plan.PrincipalID = types.Int64Value(user.PrincipalID)
	if user.DefaultSchema != "" {
		plan.DefaultSchema = types.StringValue(user.DefaultSchema)
	} else if plan.DefaultSchema.IsUnknown() {
		plan.DefaultSchema = types.StringValue("")
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *UserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state userModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	conn, err := r.factory.GetConnector(state.Server.ValueString(), state.Database.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to get connector",
			fmt.Sprintf("server %q database %q: %s", state.Server.ValueString(), state.Database.ValueString(), err),
		)
		return
	}
	defer conn.Close()

	user, err := conn.GetUser(ctx, state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to read database user",
			fmt.Sprintf("user %q in database %q on %q: %s",
				state.Name.ValueString(), state.Database.ValueString(), state.Server.ValueString(), err),
		)
		return
	}
	if user == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	state.PrincipalID = types.Int64Value(user.PrincipalID)
	// Always overwrite to match Create — keeps import state byte-equal to apply state.
	state.DefaultSchema = types.StringValue(user.DefaultSchema)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *UserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan userModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	conn, err := r.factory.GetConnector(plan.Server.ValueString(), plan.Database.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to get connector",
			fmt.Sprintf("server %q database %q: %s", plan.Server.ValueString(), plan.Database.ValueString(), err),
		)
		return
	}
	defer conn.Close()

	user := &database.User{
		Name:          plan.Name.ValueString(),
		Type:          plan.Type.ValueString(),
		DefaultSchema: plan.DefaultSchema.ValueString(),
	}

	if err := conn.UpdateUser(ctx, user); err != nil {
		resp.Diagnostics.AddError(
			"Failed to update database user",
			fmt.Sprintf("user %q in database %q on %q: %s",
				plan.Name.ValueString(), plan.Database.ValueString(), plan.Server.ValueString(), err),
		)
		return
	}

	plan.DefaultSchema = types.StringValue(user.DefaultSchema)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *UserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state userModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	conn, err := r.factory.GetConnector(state.Server.ValueString(), state.Database.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to get connector",
			fmt.Sprintf("server %q database %q: %s", state.Server.ValueString(), state.Database.ValueString(), err),
		)
		return
	}
	defer conn.Close()

	if err := conn.DeleteUser(ctx, state.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError(
			"Failed to delete database user",
			fmt.Sprintf("user %q in database %q on %q: %s",
				state.Name.ValueString(), state.Database.ValueString(), state.Server.ValueString(), err),
		)
		return
	}
}

// ImportState handles terraform import for azsqlaccess_user.
//
// Import ID format:
//
//	type = "user"              → server/database/user/name
//	type = "group"             → server/database/group/name/object_id
//	type = "service_principal" → server/database/service_principal/name/object_id
//
// object_id is part of the import ID for group and service_principal because
// it is required by the schema and cannot be read back from the database —
// omitting it would leave state in an invalid condition after import.
//
// Examples:
//
//	myserver.database.windows.net/mydb/user/juan.perez@milanesa.com
//	myserver.database.windows.net/mydb/group/db.reader/00000000-0000-0000-0000-000000000000
//	myserver.database.windows.net/mydb/service_principal/myapp-identity/00000000-0000-0000-0000-000000000000
func (r *UserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Split into at most 5 segments. UPNs and display names do not contain "/"
	// so this is unambiguous in practice.
	parts := strings.SplitN(req.ID, "/", 5)

	if len(parts) < 4 || parts[0] == "" || parts[1] == "" || parts[2] == "" || parts[3] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf(
				"expected format server/database/type/name (user) or server/database/type/name/object_id (group, service_principal), got %q",
				req.ID,
			),
		)
		return
	}

	server, database, principalType, name := parts[0], parts[1], parts[2], parts[3]

	if !strings.Contains(server, ".") {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("server %q does not look like a fully-qualified hostname (e.g. myserver.database.windows.net)", server),
		)
		return
	}

	switch principalType {
	case "user":
		if len(parts) == 5 {
			resp.Diagnostics.AddError(
				"Invalid import ID",
				"type \"user\" does not use object_id — import format is server/database/user/upn",
			)
			return
		}
		id := server + "/" + database + "/" + principalType + "/" + name
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("server"), server)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("database"), database)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("type"), principalType)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), name)...)

	case "group", "service_principal":
		if len(parts) != 5 || parts[4] == "" {
			resp.Diagnostics.AddError(
				"Invalid import ID",
				fmt.Sprintf(
					"type %q requires object_id in the import ID — format is server/database/%s/name/object_id",
					principalType, principalType,
				),
			)
			return
		}
		objectID := parts[4]
		id := server + "/" + database + "/" + principalType + "/" + name + "/" + objectID
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("server"), server)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("database"), database)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("type"), principalType)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), name)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("object_id"), objectID)...)

	default:
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("type %q is not valid — must be one of: user, group, service_principal", principalType),
		)
	}
}
