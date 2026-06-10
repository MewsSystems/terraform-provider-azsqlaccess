// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// Happy-trail acceptance tests for azsqlaccess_database_role_member.
// Six tests = 3 principal types × 2 engines. Each grants the built-in reader
// role to one principal type:
//   - MSSQL:    db_datareader
//   - Postgres: pg_read_all_data
//
// The test config declares the matching azsqlaccess_user in the same apply —
// Terraform infers ordering via the resource reference, so the user always
// exists before the role grant tries to use it. The framework destroys both
// at the end.

// ---------- MSSQL ----------------------------------------------------------

func TestAccRoleMember_user_mssql(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccRoleMemberConfig_user_mssql(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("azsqlaccess_database_role_member.test", "role", "db_datareader"),
					resource.TestCheckResourceAttr("azsqlaccess_database_role_member.test", "member", os.Getenv("AZSQLACCESS_TEST_USER_UPN")),
					resource.TestCheckResourceAttrSet("azsqlaccess_database_role_member.test", "id"),
				),
			},
			{
				ResourceName:      "azsqlaccess_database_role_member.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateId: fmt.Sprintf("%s/%s/db_datareader/%s",
					os.Getenv("AZSQLACCESS_TEST_MSSQL_SERVER"),
					os.Getenv("AZSQLACCESS_TEST_DATABASE"),
					os.Getenv("AZSQLACCESS_TEST_USER_UPN"),
				),
			},
		},
	})
}

func TestAccRoleMember_group_mssql(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccRoleMemberConfig_group_mssql(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("azsqlaccess_database_role_member.test", "role", "db_datareader"),
					resource.TestCheckResourceAttr("azsqlaccess_database_role_member.test", "member", os.Getenv("AZSQLACCESS_TEST_GROUP_NAME")),
					resource.TestCheckResourceAttrSet("azsqlaccess_database_role_member.test", "id"),
				),
			},
			{
				ResourceName:      "azsqlaccess_database_role_member.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateId: fmt.Sprintf("%s/%s/db_datareader/%s",
					os.Getenv("AZSQLACCESS_TEST_MSSQL_SERVER"),
					os.Getenv("AZSQLACCESS_TEST_DATABASE"),
					os.Getenv("AZSQLACCESS_TEST_GROUP_NAME"),
				),
			},
		},
	})
}

func TestAccRoleMember_servicePrincipal_mssql(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccRoleMemberConfig_servicePrincipal_mssql(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("azsqlaccess_database_role_member.test", "role", "db_datareader"),
					resource.TestCheckResourceAttr("azsqlaccess_database_role_member.test", "member", os.Getenv("AZSQLACCESS_TEST_SP_NAME")),
					resource.TestCheckResourceAttrSet("azsqlaccess_database_role_member.test", "id"),
				),
			},
			{
				ResourceName:      "azsqlaccess_database_role_member.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateId: fmt.Sprintf("%s/%s/db_datareader/%s",
					os.Getenv("AZSQLACCESS_TEST_MSSQL_SERVER"),
					os.Getenv("AZSQLACCESS_TEST_DATABASE"),
					os.Getenv("AZSQLACCESS_TEST_SP_NAME"),
				),
			},
		},
	})
}

// ---------- PostgreSQL -----------------------------------------------------

func TestAccRoleMember_user_postgres(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccRoleMemberConfig_user_postgres(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("azsqlaccess_database_role_member.test", "role", "pg_read_all_data"),
					resource.TestCheckResourceAttr("azsqlaccess_database_role_member.test", "member", os.Getenv("AZSQLACCESS_TEST_USER_UPN")),
					resource.TestCheckResourceAttrSet("azsqlaccess_database_role_member.test", "id"),
				),
			},
			{
				ResourceName:      "azsqlaccess_database_role_member.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateId: fmt.Sprintf("%s/%s/pg_read_all_data/%s",
					os.Getenv("AZSQLACCESS_TEST_POSTGRES_SERVER"),
					os.Getenv("AZSQLACCESS_TEST_POSTGRES_DATABASE"),
					os.Getenv("AZSQLACCESS_TEST_USER_UPN"),
				),
			},
		},
	})
}

func TestAccRoleMember_group_postgres(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccRoleMemberConfig_group_postgres(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("azsqlaccess_database_role_member.test", "role", "pg_read_all_data"),
					resource.TestCheckResourceAttr("azsqlaccess_database_role_member.test", "member", os.Getenv("AZSQLACCESS_TEST_GROUP_NAME")),
					resource.TestCheckResourceAttrSet("azsqlaccess_database_role_member.test", "id"),
				),
			},
			{
				ResourceName:      "azsqlaccess_database_role_member.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateId: fmt.Sprintf("%s/%s/pg_read_all_data/%s",
					os.Getenv("AZSQLACCESS_TEST_POSTGRES_SERVER"),
					os.Getenv("AZSQLACCESS_TEST_POSTGRES_DATABASE"),
					os.Getenv("AZSQLACCESS_TEST_GROUP_NAME"),
				),
			},
		},
	})
}

func TestAccRoleMember_servicePrincipal_postgres(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccRoleMemberConfig_servicePrincipal_postgres(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("azsqlaccess_database_role_member.test", "role", "pg_read_all_data"),
					resource.TestCheckResourceAttr("azsqlaccess_database_role_member.test", "member", os.Getenv("AZSQLACCESS_TEST_SP_NAME")),
					resource.TestCheckResourceAttrSet("azsqlaccess_database_role_member.test", "id"),
				),
			},
			{
				ResourceName:      "azsqlaccess_database_role_member.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateId: fmt.Sprintf("%s/%s/pg_read_all_data/%s",
					os.Getenv("AZSQLACCESS_TEST_POSTGRES_SERVER"),
					os.Getenv("AZSQLACCESS_TEST_POSTGRES_DATABASE"),
					os.Getenv("AZSQLACCESS_TEST_SP_NAME"),
				),
			},
		},
	})
}

// ---------- HCL config builders --------------------------------------------

func testAccRoleMemberConfig_user_mssql() string {
	return mssqlProviderConfig + fmt.Sprintf(`
resource "azsqlaccess_user" "test" {
  server   = %q
  database = %q
  type     = "user"
  name     = %q
}

resource "azsqlaccess_database_role_member" "test" {
  server   = %q
  database = %q
  role     = "db_datareader"
  member   = azsqlaccess_user.test.name
}
`,
		os.Getenv("AZSQLACCESS_TEST_MSSQL_SERVER"),
		os.Getenv("AZSQLACCESS_TEST_DATABASE"),
		os.Getenv("AZSQLACCESS_TEST_USER_UPN"),
		os.Getenv("AZSQLACCESS_TEST_MSSQL_SERVER"),
		os.Getenv("AZSQLACCESS_TEST_DATABASE"),
	)
}

func testAccRoleMemberConfig_group_mssql() string {
	return mssqlProviderConfig + fmt.Sprintf(`
resource "azsqlaccess_user" "test" {
  server    = %q
  database  = %q
  type      = "group"
  name      = %q
  object_id = %q
}

resource "azsqlaccess_database_role_member" "test" {
  server   = %q
  database = %q
  role     = "db_datareader"
  member   = azsqlaccess_user.test.name
}
`,
		os.Getenv("AZSQLACCESS_TEST_MSSQL_SERVER"),
		os.Getenv("AZSQLACCESS_TEST_DATABASE"),
		os.Getenv("AZSQLACCESS_TEST_GROUP_NAME"),
		os.Getenv("AZSQLACCESS_TEST_GROUP_OBJECT_ID"),
		os.Getenv("AZSQLACCESS_TEST_MSSQL_SERVER"),
		os.Getenv("AZSQLACCESS_TEST_DATABASE"),
	)
}

func testAccRoleMemberConfig_servicePrincipal_mssql() string {
	return mssqlProviderConfig + fmt.Sprintf(`
resource "azsqlaccess_user" "test" {
  server    = %q
  database  = %q
  type      = "service_principal"
  name      = %q
  object_id = %q
}

resource "azsqlaccess_database_role_member" "test" {
  server   = %q
  database = %q
  role     = "db_datareader"
  member   = azsqlaccess_user.test.name
}
`,
		os.Getenv("AZSQLACCESS_TEST_MSSQL_SERVER"),
		os.Getenv("AZSQLACCESS_TEST_DATABASE"),
		os.Getenv("AZSQLACCESS_TEST_SP_NAME"),
		os.Getenv("AZSQLACCESS_TEST_SP_OBJECT_ID"),
		os.Getenv("AZSQLACCESS_TEST_MSSQL_SERVER"),
		os.Getenv("AZSQLACCESS_TEST_DATABASE"),
	)
}

func testAccRoleMemberConfig_user_postgres() string {
	return postgresProviderConfig + fmt.Sprintf(`
resource "azsqlaccess_user" "test" {
  server   = %q
  database = %q
  type     = "user"
  name     = %q
}

resource "azsqlaccess_database_role_member" "test" {
  server   = %q
  database = %q
  role     = "pg_read_all_data"
  member   = azsqlaccess_user.test.name
}
`,
		os.Getenv("AZSQLACCESS_TEST_POSTGRES_SERVER"),
		os.Getenv("AZSQLACCESS_TEST_POSTGRES_DATABASE"),
		os.Getenv("AZSQLACCESS_TEST_USER_UPN"),
		os.Getenv("AZSQLACCESS_TEST_POSTGRES_SERVER"),
		os.Getenv("AZSQLACCESS_TEST_POSTGRES_DATABASE"),
	)
}

func testAccRoleMemberConfig_group_postgres() string {
	return postgresProviderConfig + fmt.Sprintf(`
resource "azsqlaccess_user" "test" {
  server    = %q
  database  = %q
  type      = "group"
  name      = %q
  object_id = %q
}

resource "azsqlaccess_database_role_member" "test" {
  server   = %q
  database = %q
  role     = "pg_read_all_data"
  member   = azsqlaccess_user.test.name
}
`,
		os.Getenv("AZSQLACCESS_TEST_POSTGRES_SERVER"),
		os.Getenv("AZSQLACCESS_TEST_POSTGRES_DATABASE"),
		os.Getenv("AZSQLACCESS_TEST_GROUP_NAME"),
		os.Getenv("AZSQLACCESS_TEST_GROUP_OBJECT_ID"),
		os.Getenv("AZSQLACCESS_TEST_POSTGRES_SERVER"),
		os.Getenv("AZSQLACCESS_TEST_POSTGRES_DATABASE"),
	)
}

func testAccRoleMemberConfig_servicePrincipal_postgres() string {
	return postgresProviderConfig + fmt.Sprintf(`
resource "azsqlaccess_user" "test" {
  server    = %q
  database  = %q
  type      = "service_principal"
  name      = %q
  object_id = %q
}

resource "azsqlaccess_database_role_member" "test" {
  server   = %q
  database = %q
  role     = "pg_read_all_data"
  member   = azsqlaccess_user.test.name
}
`,
		os.Getenv("AZSQLACCESS_TEST_POSTGRES_SERVER"),
		os.Getenv("AZSQLACCESS_TEST_POSTGRES_DATABASE"),
		os.Getenv("AZSQLACCESS_TEST_SP_NAME"),
		os.Getenv("AZSQLACCESS_TEST_SP_OBJECT_ID"),
		os.Getenv("AZSQLACCESS_TEST_POSTGRES_SERVER"),
		os.Getenv("AZSQLACCESS_TEST_POSTGRES_DATABASE"),
	)
}
