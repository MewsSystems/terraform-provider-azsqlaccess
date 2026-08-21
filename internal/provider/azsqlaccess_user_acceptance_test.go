// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// Happy-trail acceptance tests for azsqlaccess_user.
// One test per (principal type) × (engine) combination, 6 tests total.
// Each test:
//   - Applies the HCL → provider issues CREATE USER ... FROM EXTERNAL PROVIDER
//     (MSSQL) or pgaadauth_create_principal(...) (Postgres) against the
//     ephemeral fixture
//   - Reads back state attrs, including principal_id (proves the DB row exists)
//   - Auto-destroys at the end of the TestCase
//
// Gated by TF_ACC=1 — `go test ./...` without that env var skips entirely.

// ---------- MSSQL ----------------------------------------------------------

func TestAccUser_user_mssql(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccUserConfig_user_mssql(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("azsqlaccess_user.test", "type", "user"),
					resource.TestCheckResourceAttr("azsqlaccess_user.test", "name", os.Getenv("AZSQLACCESS_TEST_USER_UPN")),
					resource.TestCheckResourceAttrSet("azsqlaccess_user.test", "principal_id"),
					resource.TestCheckResourceAttrSet("azsqlaccess_user.test", "id"),
				),
			},
			{
				ResourceName:      "azsqlaccess_user.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateId: fmt.Sprintf("%s/%s/user/%s",
					os.Getenv("AZSQLACCESS_TEST_MSSQL_SERVER"),
					os.Getenv("AZSQLACCESS_TEST_DATABASE"),
					os.Getenv("AZSQLACCESS_TEST_USER_UPN"),
				),
			},
		},
	})
}

func TestAccUser_group_mssql(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccUserConfig_group_mssql(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("azsqlaccess_user.test", "type", "group"),
					resource.TestCheckResourceAttr("azsqlaccess_user.test", "name", os.Getenv("AZSQLACCESS_TEST_GROUP_NAME")),
					resource.TestCheckResourceAttr("azsqlaccess_user.test", "object_id", os.Getenv("AZSQLACCESS_TEST_GROUP_OBJECT_ID")),
					resource.TestCheckResourceAttrSet("azsqlaccess_user.test", "principal_id"),
				),
			},
			{
				ResourceName:      "azsqlaccess_user.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateId: fmt.Sprintf("%s/%s/group/%s/%s",
					os.Getenv("AZSQLACCESS_TEST_MSSQL_SERVER"),
					os.Getenv("AZSQLACCESS_TEST_DATABASE"),
					os.Getenv("AZSQLACCESS_TEST_GROUP_NAME"),
					os.Getenv("AZSQLACCESS_TEST_GROUP_OBJECT_ID"),
				),
			},
		},
	})
}

func TestAccUser_servicePrincipal_mssql(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccUserConfig_servicePrincipal_mssql(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("azsqlaccess_user.test", "type", "service_principal"),
					resource.TestCheckResourceAttr("azsqlaccess_user.test", "name", os.Getenv("AZSQLACCESS_TEST_SP_NAME")),
					resource.TestCheckResourceAttr("azsqlaccess_user.test", "object_id", os.Getenv("AZSQLACCESS_TEST_SP_OBJECT_ID")),
					resource.TestCheckResourceAttrSet("azsqlaccess_user.test", "principal_id"),
				),
			},
			{
				ResourceName:      "azsqlaccess_user.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateId: fmt.Sprintf("%s/%s/service_principal/%s/%s",
					os.Getenv("AZSQLACCESS_TEST_MSSQL_SERVER"),
					os.Getenv("AZSQLACCESS_TEST_DATABASE"),
					os.Getenv("AZSQLACCESS_TEST_SP_NAME"),
					os.Getenv("AZSQLACCESS_TEST_SP_OBJECT_ID"),
				),
			},
		},
	})
}

// ---------- PostgreSQL -----------------------------------------------------

func TestAccUser_user_postgres(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccUserConfig_user_postgres(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("azsqlaccess_user.test", "type", "user"),
					resource.TestCheckResourceAttr("azsqlaccess_user.test", "name", os.Getenv("AZSQLACCESS_TEST_USER_UPN")),
					resource.TestCheckResourceAttrSet("azsqlaccess_user.test", "principal_id"),
					resource.TestCheckResourceAttrSet("azsqlaccess_user.test", "id"),
				),
			},
			{
				ResourceName:      "azsqlaccess_user.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateId: fmt.Sprintf("%s/%s/user/%s",
					os.Getenv("AZSQLACCESS_TEST_POSTGRES_SERVER"),
					os.Getenv("AZSQLACCESS_TEST_POSTGRES_DATABASE"),
					os.Getenv("AZSQLACCESS_TEST_USER_UPN"),
				),
			},
		},
	})
}

func TestAccUser_group_postgres(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccUserConfig_group_postgres(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("azsqlaccess_user.test", "type", "group"),
					resource.TestCheckResourceAttr("azsqlaccess_user.test", "name", os.Getenv("AZSQLACCESS_TEST_GROUP_NAME")),
					resource.TestCheckResourceAttr("azsqlaccess_user.test", "object_id", os.Getenv("AZSQLACCESS_TEST_GROUP_OBJECT_ID")),
					resource.TestCheckResourceAttrSet("azsqlaccess_user.test", "principal_id"),
				),
			},
			{
				ResourceName:      "azsqlaccess_user.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateId: fmt.Sprintf("%s/%s/group/%s/%s",
					os.Getenv("AZSQLACCESS_TEST_POSTGRES_SERVER"),
					os.Getenv("AZSQLACCESS_TEST_POSTGRES_DATABASE"),
					os.Getenv("AZSQLACCESS_TEST_GROUP_NAME"),
					os.Getenv("AZSQLACCESS_TEST_GROUP_OBJECT_ID"),
				),
			},
		},
	})
}

func TestAccUser_servicePrincipal_postgres(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccUserConfig_servicePrincipal_postgres(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("azsqlaccess_user.test", "type", "service_principal"),
					resource.TestCheckResourceAttr("azsqlaccess_user.test", "name", os.Getenv("AZSQLACCESS_TEST_SP_NAME")),
					resource.TestCheckResourceAttr("azsqlaccess_user.test", "object_id", os.Getenv("AZSQLACCESS_TEST_SP_OBJECT_ID")),
					resource.TestCheckResourceAttrSet("azsqlaccess_user.test", "principal_id"),
				),
			},
			{
				ResourceName:      "azsqlaccess_user.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateId: fmt.Sprintf("%s/%s/service_principal/%s/%s",
					os.Getenv("AZSQLACCESS_TEST_POSTGRES_SERVER"),
					os.Getenv("AZSQLACCESS_TEST_POSTGRES_DATABASE"),
					os.Getenv("AZSQLACCESS_TEST_SP_NAME"),
					os.Getenv("AZSQLACCESS_TEST_SP_OBJECT_ID"),
				),
			},
		},
	})
}

// ---------- Deferred object_id ---------------------------------------------

// The tests above hardcode object_id, so it is always known during the validate
// walk. These two source it from an azuread data source instead, which is
// unknown at that point — the state that used to trip ValidateConfig's
// "object_id is required" error before it learned to distinguish unknown from
// null. Postgres only: the check lives in internal/resources/user, above the
// connector layer, so it is engine-independent. Both types that require
// object_id are covered because they share one switch arm.

// azureadProvider pins the external provider used to resolve object IDs at plan
// time. Requires directory read permission (Group.Read.All / Application.Read.All)
// on whatever principal the acceptance run authenticates as.
var azureadProvider = map[string]resource.ExternalProvider{
	"azuread": {Source: "hashicorp/azuread", VersionConstraint: "~> 3.0"},
}

func TestAccUser_groupDeferredObjectID_postgres(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		ExternalProviders:        azureadProvider,
		Steps: []resource.TestStep{
			{
				Config: testAccUserConfig_groupDeferredObjectID_postgres(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("azsqlaccess_user.test", "type", "group"),
					resource.TestCheckResourceAttr("azsqlaccess_user.test", "name", os.Getenv("AZSQLACCESS_TEST_GROUP_NAME")),
					resource.TestCheckResourceAttr("azsqlaccess_user.test", "object_id", os.Getenv("AZSQLACCESS_TEST_GROUP_OBJECT_ID")),
					resource.TestCheckResourceAttrSet("azsqlaccess_user.test", "principal_id"),
				),
			},
		},
	})
}

func TestAccUser_servicePrincipalDeferredObjectID_postgres(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		ExternalProviders:        azureadProvider,
		Steps: []resource.TestStep{
			{
				Config: testAccUserConfig_servicePrincipalDeferredObjectID_postgres(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("azsqlaccess_user.test", "type", "service_principal"),
					resource.TestCheckResourceAttr("azsqlaccess_user.test", "name", os.Getenv("AZSQLACCESS_TEST_SP_NAME")),
					resource.TestCheckResourceAttr("azsqlaccess_user.test", "object_id", os.Getenv("AZSQLACCESS_TEST_SP_OBJECT_ID")),
					resource.TestCheckResourceAttrSet("azsqlaccess_user.test", "principal_id"),
				),
			},
		},
	})
}

// ---------- HCL config builders --------------------------------------------

const mssqlProviderConfig = `
provider "azsqlaccess" {
  engine = "mssql"
}
`

const postgresProviderConfig = `
provider "azsqlaccess" {
  engine = "postgres"
}
`

func testAccUserConfig_user_mssql() string {
	return mssqlProviderConfig + fmt.Sprintf(`
resource "azsqlaccess_user" "test" {
  server   = %q
  database = %q
  type     = "user"
  name     = %q
}
`,
		os.Getenv("AZSQLACCESS_TEST_MSSQL_SERVER"),
		os.Getenv("AZSQLACCESS_TEST_DATABASE"),
		os.Getenv("AZSQLACCESS_TEST_USER_UPN"),
	)
}

func testAccUserConfig_group_mssql() string {
	return mssqlProviderConfig + fmt.Sprintf(`
resource "azsqlaccess_user" "test" {
  server    = %q
  database  = %q
  type      = "group"
  name      = %q
  object_id = %q
}
`,
		os.Getenv("AZSQLACCESS_TEST_MSSQL_SERVER"),
		os.Getenv("AZSQLACCESS_TEST_DATABASE"),
		os.Getenv("AZSQLACCESS_TEST_GROUP_NAME"),
		os.Getenv("AZSQLACCESS_TEST_GROUP_OBJECT_ID"),
	)
}

func testAccUserConfig_servicePrincipal_mssql() string {
	return mssqlProviderConfig + fmt.Sprintf(`
resource "azsqlaccess_user" "test" {
  server    = %q
  database  = %q
  type      = "service_principal"
  name      = %q
  object_id = %q
}
`,
		os.Getenv("AZSQLACCESS_TEST_MSSQL_SERVER"),
		os.Getenv("AZSQLACCESS_TEST_DATABASE"),
		os.Getenv("AZSQLACCESS_TEST_SP_NAME"),
		os.Getenv("AZSQLACCESS_TEST_SP_OBJECT_ID"),
	)
}

func testAccUserConfig_user_postgres() string {
	return postgresProviderConfig + fmt.Sprintf(`
resource "azsqlaccess_user" "test" {
  server   = %q
  database = %q
  type     = "user"
  name     = %q
}
`,
		os.Getenv("AZSQLACCESS_TEST_POSTGRES_SERVER"),
		os.Getenv("AZSQLACCESS_TEST_POSTGRES_DATABASE"),
		os.Getenv("AZSQLACCESS_TEST_USER_UPN"),
	)
}

func testAccUserConfig_group_postgres() string {
	return postgresProviderConfig + fmt.Sprintf(`
resource "azsqlaccess_user" "test" {
  server    = %q
  database  = %q
  type      = "group"
  name      = %q
  object_id = %q
}
`,
		os.Getenv("AZSQLACCESS_TEST_POSTGRES_SERVER"),
		os.Getenv("AZSQLACCESS_TEST_POSTGRES_DATABASE"),
		os.Getenv("AZSQLACCESS_TEST_GROUP_NAME"),
		os.Getenv("AZSQLACCESS_TEST_GROUP_OBJECT_ID"),
	)
}

func testAccUserConfig_servicePrincipal_postgres() string {
	return postgresProviderConfig + fmt.Sprintf(`
resource "azsqlaccess_user" "test" {
  server    = %q
  database  = %q
  type      = "service_principal"
  name      = %q
  object_id = %q
}
`,
		os.Getenv("AZSQLACCESS_TEST_POSTGRES_SERVER"),
		os.Getenv("AZSQLACCESS_TEST_POSTGRES_DATABASE"),
		os.Getenv("AZSQLACCESS_TEST_SP_NAME"),
		os.Getenv("AZSQLACCESS_TEST_SP_OBJECT_ID"),
	)
}

func testAccUserConfig_groupDeferredObjectID_postgres() string {
	return postgresProviderConfig + fmt.Sprintf(`
data "azuread_group" "test" {
  display_name = %q
}

resource "azsqlaccess_user" "test" {
  server    = %q
  database  = %q
  type      = "group"
  name      = data.azuread_group.test.display_name
  object_id = data.azuread_group.test.object_id
}
`,
		os.Getenv("AZSQLACCESS_TEST_GROUP_NAME"),
		os.Getenv("AZSQLACCESS_TEST_POSTGRES_SERVER"),
		os.Getenv("AZSQLACCESS_TEST_POSTGRES_DATABASE"),
	)
}

func testAccUserConfig_servicePrincipalDeferredObjectID_postgres() string {
	return postgresProviderConfig + fmt.Sprintf(`
data "azuread_service_principal" "test" {
  display_name = %q
}

resource "azsqlaccess_user" "test" {
  server    = %q
  database  = %q
  type      = "service_principal"
  name      = data.azuread_service_principal.test.display_name
  object_id = data.azuread_service_principal.test.object_id
}
`,
		os.Getenv("AZSQLACCESS_TEST_SP_NAME"),
		os.Getenv("AZSQLACCESS_TEST_POSTGRES_SERVER"),
		os.Getenv("AZSQLACCESS_TEST_POSTGRES_DATABASE"),
	)
}

// ---------- login_username (group login, PostgreSQL) ------------------------

// TestAccUser_user_postgres_viaGroupLogin exercises the provider's
// login_username against a real server: it connects as the Entra group's own
// role rather than as the caller, which is the only way a caller whose admin
// rights come solely from group membership can authenticate to PostgreSQL
// Flexible Server (pgaadauth performs no server-side group expansion).
//
// Skipped unless AZSQLACCESS_TEST_POSTGRES_ADMIN_GROUP names a group that is
// configured as an Entra administrator on the test server AND contains the
// identity running the test — a setup the other tests don't require, so it is
// opt-in rather than enforced in testAccPreCheck.
func TestAccUser_user_postgres_viaGroupLogin(t *testing.T) {
	adminGroup := os.Getenv("AZSQLACCESS_TEST_POSTGRES_ADMIN_GROUP")
	if adminGroup == "" {
		t.Skip("AZSQLACCESS_TEST_POSTGRES_ADMIN_GROUP not set; skipping group-login test")
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccUserConfig_user_postgres_viaGroupLogin(adminGroup),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("azsqlaccess_user.test", "type", "user"),
					resource.TestCheckResourceAttr("azsqlaccess_user.test", "name", os.Getenv("AZSQLACCESS_TEST_USER_UPN")),
					resource.TestCheckResourceAttrSet("azsqlaccess_user.test", "principal_id"),
				),
			},
		},
	})
}

// TestAccProvider_loginUsername_rejectedOnMSSQL asserts the ValidateConfig
// guard fires before any connection is attempted. It reaches no Azure resource,
// but lives here because only the acceptance harness runs real config
// validation through the plugin protocol.
func TestAccProvider_loginUsername_rejectedOnMSSQL(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccProviderConfig_loginUsername_mssql(),
				ExpectError: regexp.MustCompile(`login_username is not supported`),
			},
		},
	})
}

func testAccUserConfig_user_postgres_viaGroupLogin(adminGroup string) string {
	return fmt.Sprintf(`
provider "azsqlaccess" {
  engine         = "postgres"
  login_username = %q
}

resource "azsqlaccess_user" "test" {
  server   = %q
  database = %q
  type     = "user"
  name     = %q
}
`,
		adminGroup,
		os.Getenv("AZSQLACCESS_TEST_POSTGRES_SERVER"),
		os.Getenv("AZSQLACCESS_TEST_POSTGRES_DATABASE"),
		os.Getenv("AZSQLACCESS_TEST_USER_UPN"),
	)
}

func testAccProviderConfig_loginUsername_mssql() string {
	return fmt.Sprintf(`
provider "azsqlaccess" {
  engine         = "mssql"
  login_username = "db.reader"
}

resource "azsqlaccess_user" "test" {
  server   = %q
  database = %q
  type     = "user"
  name     = %q
}
`,
		os.Getenv("AZSQLACCESS_TEST_MSSQL_SERVER"),
		os.Getenv("AZSQLACCESS_TEST_DATABASE"),
		os.Getenv("AZSQLACCESS_TEST_USER_UPN"),
	)
}
