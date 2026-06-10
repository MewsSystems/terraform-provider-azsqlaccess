// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"os"
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
