// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package role_member

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/mews/terraform-provider-azsqlaccess/internal/database"
	"github.com/mews/terraform-provider-azsqlaccess/internal/resources/dbscope"
)

var (
	_ resource.Resource                = &RoleMemberResource{}
	_ resource.ResourceWithConfigure   = &RoleMemberResource{}
	_ resource.ResourceWithImportState = &RoleMemberResource{}
	_ resource.ResourceWithModifyPlan  = &RoleMemberResource{}
)

// RoleMemberResource manages an azsqlaccess_database_role_member resource.
type RoleMemberResource struct {
	factory database.ConnectorFactory
}

func NewResource() resource.Resource {
	return &RoleMemberResource{}
}

func (r *RoleMemberResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_database_role_member"
}

func (r *RoleMemberResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Grants a database role to a member (user, group, or managed identity). " +
			"Changing `role` or `member` destroys and recreates the resource.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"server": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Fully-qualified server hostname (e.g. `myserver.database.windows.net` for Azure SQL, `myserver.postgres.database.azure.com` for PostgreSQL).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"database": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Database the provider connects through. On Azure SQL the membership lives in this database, and changing it forces resource replacement. On PostgreSQL role membership belongs to the server, so changing it only changes where the provider connects and is applied in place.",
			},
			"role": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Name of the database role to grant. Role names are engine-specific:\n\n" +
					"| MSSQL | PostgreSQL | Description |\n" +
					"|---|---|---|\n" +
					"| `db_datareader` | `pg_read_all_data` | Read all tables |\n" +
					"| `db_datawriter` | `pg_write_all_data` | Write all tables |\n" +
					"| `db_owner` | `pg_database_owner` | Full database ownership |\n" +
					"| `db_ddladmin` | *(no direct equivalent)* | DDL operations |\n\n" +
					"Changing this forces resource replacement.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"member": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Principal name of the member to add to the role — must match the `name` of an `azsqlaccess_user` resource.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *RoleMemberResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	factory, ok := req.ProviderData.(database.ConnectorFactory)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data type",
			fmt.Sprintf("expected database.ConnectorFactory, got %T", req.ProviderData),
		)
		return
	}
	r.factory = factory
}

func (r *RoleMemberResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan roleMemberModel
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

	rm := &database.RoleMember{
		Role:   plan.Role.ValueString(),
		Member: plan.Member.ValueString(),
	}

	if err := conn.CreateRoleMember(ctx, rm); err != nil {
		resp.Diagnostics.AddError(
			"Failed to create role membership",
			fmt.Sprintf("role %q member %q in database %q on %q: %s",
				plan.Role.ValueString(), plan.Member.ValueString(),
				plan.Database.ValueString(), plan.Server.ValueString(), err),
		)
		return
	}

	plan.ID = types.StringValue(roleMemberID(plan))

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *RoleMemberResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state roleMemberModel
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

	rm, err := conn.GetRoleMember(ctx, state.Role.ValueString(), state.Member.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to read role membership",
			fmt.Sprintf("role %q member %q in database %q on %q: %s",
				state.Role.ValueString(), state.Member.ValueString(),
				state.Database.ValueString(), state.Server.ValueString(), err),
		)
		return
	}
	if rm == nil {
		// Membership was removed outside Terraform — signal recreate.
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update runs only for a database change on a server-scoped engine: the membership is
// unchanged, so it is confirmed through the new database and the id recomputed.
func (r *RoleMemberResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan roleMemberModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	conn, err := r.factory.GetConnector(plan.Server.ValueString(), plan.Database.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to get connector",
			fmt.Sprintf("server %q database %q: %s", plan.Server.ValueString(), plan.Database.ValueString(), err))
		return
	}
	defer conn.Close()
	rm, err := conn.GetRoleMember(ctx, plan.Role.ValueString(), plan.Member.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read role membership",
			fmt.Sprintf("role %q member %q in database %q on %q: %s", plan.Role.ValueString(),
				plan.Member.ValueString(), plan.Database.ValueString(), plan.Server.ValueString(), err))
		return
	}
	if rm == nil {
		resp.Diagnostics.AddError("Role membership not found",
			fmt.Sprintf("role %q member %q is not visible from database %q on %q", plan.Role.ValueString(),
				plan.Member.ValueString(), plan.Database.ValueString(), plan.Server.ValueString()))
		return
	}
	plan.ID = types.StringValue(roleMemberID(plan))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// ModifyPlan plans a database change in place or as a replacement, per engine.
func (r *RoleMemberResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	var state, plan roleMemberModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || plan.Database.Equal(state.Database) {
		return
	}
	dbscope.PlanDatabaseChange(ctx, r.factory, resp)
}

// roleMemberID mirrors the import ID: server/database/role/member.
func roleMemberID(m roleMemberModel) string {
	return strings.Join([]string{m.Server.ValueString(), m.Database.ValueString(), m.Role.ValueString(), m.Member.ValueString()}, "/")
}

func (r *RoleMemberResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state roleMemberModel
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

	if err := conn.DeleteRoleMember(ctx, state.Role.ValueString(), state.Member.ValueString()); err != nil {
		resp.Diagnostics.AddError(
			"Failed to delete role membership",
			fmt.Sprintf("role %q member %q in database %q on %q: %s",
				state.Role.ValueString(), state.Member.ValueString(),
				state.Database.ValueString(), state.Server.ValueString(), err),
		)
	}
}

// ImportState handles `terraform import azsqlaccess_database_role_member.x server/database/role/member`.
func (r *RoleMemberResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 4)
	if len(parts) != 4 || parts[0] == "" || parts[1] == "" || parts[2] == "" || parts[3] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("expected format server/database/role/member, got %q", req.ID),
		)
		return
	}
	// Catch obviously wrong server values early — a valid hostname always has at
	// least one dot (e.g. "myserver.database.windows.net").
	if !strings.Contains(parts[0], ".") {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("server %q does not look like a fully-qualified hostname (e.g. myserver.database.windows.net)", parts[0]),
		)
		return
	}

	state := roleMemberModel{
		ID:       types.StringValue(req.ID),
		Server:   types.StringValue(parts[0]),
		Database: types.StringValue(parts[1]),
		Role:     types.StringValue(parts[2]),
		Member:   types.StringValue(parts[3]),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
