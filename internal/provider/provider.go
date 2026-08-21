// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"cmp"
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/mews/terraform-provider-azsqlaccess/internal/database"
	"github.com/mews/terraform-provider-azsqlaccess/internal/database/mssql"
	"github.com/mews/terraform-provider-azsqlaccess/internal/database/postgres"
	"github.com/mews/terraform-provider-azsqlaccess/internal/resources/role_member"
	"github.com/mews/terraform-provider-azsqlaccess/internal/resources/user"
	"github.com/mews/terraform-provider-azsqlaccess/internal/validators"
)

var _ provider.Provider = &AzsqlaccessProvider{}
var _ provider.ProviderWithValidateConfig = &AzsqlaccessProvider{}

type AzsqlaccessProvider struct {
	version string
}

// providerModel maps the HCL provider block to a typed Go struct.
// Server is per-resource — one Entra identity can access multiple servers.
type providerModel struct {
	Engine        types.String `tfsdk:"engine"`
	TenantID      types.String `tfsdk:"tenant_id"`
	ClientID      types.String `tfsdk:"client_id"`
	ClientSecret  types.String `tfsdk:"client_secret"`
	LoginUsername types.String `tfsdk:"login_username"`
}

func (p *AzsqlaccessProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "azsqlaccess"
	resp.Version = p.version
}

func (p *AzsqlaccessProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages contained database users and role memberships via Entra ID (Azure AD) " +
			"on Azure SQL and Azure PostgreSQL Flexible Server. SQL Server auth is not supported — Entra ID only.\n\n" +
			"The `engine` attribute selects the database engine. All other provider attributes configure " +
			"the Entra credential used to connect. Resources (`azsqlaccess_user`, " +
			"`azsqlaccess_database_role_member`) are engine-agnostic — the same HCL works for both engines.",
		Attributes: map[string]schema.Attribute{
			"engine": schema.StringAttribute{
				MarkdownDescription: "Database engine to connect to. Must be `\"mssql\"` (Azure SQL) or " +
					"`\"postgres\"` (Azure PostgreSQL Flexible Server).\n\n" +
					"Use provider aliases when managing both engines in the same root module:\n\n" +
					"```hcl\n" +
					"provider \"azsqlaccess\" { engine = \"mssql\" }\n" +
					"provider \"azsqlaccess\" { alias = \"postgres\"; engine = \"postgres\" }\n" +
					"```",
				Required: true,
				Validators: []validator.String{
					validators.StringOneOf("mssql", "postgres"),
				},
			},
			"tenant_id": schema.StringAttribute{
				MarkdownDescription: "Entra tenant ID. Falls back to `AZURE_TENANT_ID` then `ARM_TENANT_ID`. " +
					"Required for service-principal and OIDC auth modes.",
				Optional: true,
			},
			"client_id": schema.StringAttribute{
				MarkdownDescription: "Service principal client ID (application ID). Falls back to `AZURE_CLIENT_ID` " +
					"then `ARM_CLIENT_ID`. Required for service-principal and OIDC auth modes.",
				Optional: true,
			},
			"client_secret": schema.StringAttribute{
				MarkdownDescription: "Service principal client secret. Falls back to `AZURE_CLIENT_SECRET` " +
					"then `ARM_CLIENT_SECRET`. Sensitive — never written to plan output. " +
					"Set together with `tenant_id` and `client_id` to use client-secret auth; otherwise the " +
					"provider falls through to OIDC (when `ARM_USE_OIDC=true`) or the ambient chain " +
					"(Azure CLI → Workload Identity → Managed Identity).",
				Optional:  true,
				Sensitive: true,
			},
			"login_username": schema.StringAttribute{
				MarkdownDescription: "PostgreSQL role to connect as, overriding the identity derived from the " +
					"Entra token. Falls back to `AZSQLACCESS_LOGIN_USERNAME`. **`engine = \"postgres\"` only** — " +
					"Azure SQL negotiates the principal over federated auth and sends no username, so setting " +
					"this with `engine = \"mssql\"` is an error.\n\n" +
					"Set this when the caller's administrator rights come from **Entra group membership**. " +
					"PostgreSQL Flexible Server does not expand groups server-side: it matches the token " +
					"against a role that already exists, and that role is whichever name the connection asks " +
					"for. A group member must therefore connect as the group's own role name, presenting its " +
					"own token as the password:\n\n" +
					"```hcl\n" +
					"provider \"azsqlaccess\" {\n" +
					"  engine         = \"postgres\"\n" +
					"  login_username = \"db.reader\" # the Entra group configured as server administrator\n" +
					"}\n" +
					"```\n\n" +
					"Leave unset to connect as the caller itself — its UPN for a user, its client ID for a " +
					"service principal or managed identity — which requires that principal to be an " +
					"administrator in its own right.",
				Optional: true,
			},
		},
	}
}

// ValidateConfig rejects login_username on the mssql engine. The MSSQL connector
// authenticates through go-mssqldb's access-token connector, which puts no
// username on the wire at all — the server resolves the principal (and expands
// its group membership) from the token itself. Accepting the attribute there
// would silently do nothing.
//
// Only the HCL attribute is checked: AZSQLACCESS_LOGIN_USERNAME is process-wide,
// so in a module running both engines it would fail the mssql instance over a
// variable exported for the postgres one.
func (p *AzsqlaccessProvider) ValidateConfig(ctx context.Context, req provider.ValidateConfigRequest, resp *provider.ValidateConfigResponse) {
	var config providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Unknown values are re-validated once they resolve.
	if config.Engine.IsUnknown() || config.LoginUsername.IsUnknown() {
		return
	}

	if config.Engine.ValueString() == "mssql" && config.LoginUsername.ValueString() != "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("login_username"),
			"login_username is not supported with engine = \"mssql\"",
			"Azure SQL authenticates via federated auth and sends no username — the server derives the "+
				"principal from the token and expands its Entra group membership itself. Remove "+
				"login_username, and make sure the caller is a member of a group configured as an Entra "+
				"administrator on the server.",
		)
	}
}

// Configure runs once at startup. It builds an engine-agnostic Entra credential
// first, then selects the engine-specific ConnectorFactory based on the engine
// attribute. Adding a new engine means adding a case here — nothing else changes.
func (p *AzsqlaccessProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tenantID := cmp.Or(config.TenantID.ValueString(), os.Getenv("AZURE_TENANT_ID"), os.Getenv("ARM_TENANT_ID"))
	clientID := cmp.Or(config.ClientID.ValueString(), os.Getenv("AZURE_CLIENT_ID"), os.Getenv("ARM_CLIENT_ID"))
	clientSecret := cmp.Or(config.ClientSecret.ValueString(), os.Getenv("AZURE_CLIENT_SECRET"), os.Getenv("ARM_CLIENT_SECRET"))
	loginUsername := cmp.Or(config.LoginUsername.ValueString(), os.Getenv("AZSQLACCESS_LOGIN_USERNAME"))

	// Single Entra credential, shared by both engines. Precedence inside
	// BuildEntraCredential: explicit SP → ARM_USE_OIDC → ambient chain
	// (Azure CLI → Workload Identity → Managed Identity).
	cred, err := database.BuildEntraCredential(tenantID, clientID, clientSecret)
	if err != nil {
		resp.Diagnostics.AddError("Failed to initialise Azure credential", err.Error())
		return
	}

	switch config.Engine.ValueString() {
	case "mssql":
		resp.ResourceData = mssql.NewFactory(cred)
	case "postgres":
		resp.ResourceData = postgres.NewFactory(cred, loginUsername)
	default:
		resp.Diagnostics.AddError(
			"Unsupported engine",
			`engine must be "mssql" or "postgres"`,
		)
	}
}

func (p *AzsqlaccessProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		user.NewResource,
		role_member.NewResource,
	}
}

func (p *AzsqlaccessProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return nil
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &AzsqlaccessProvider{version: version}
	}
}
