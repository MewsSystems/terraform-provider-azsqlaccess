// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mews/terraform-provider-azsqlaccess/internal/database"
	"github.com/pashagolub/pgxmock/v4"
)

const (
	checkPgaadSQL = "SELECT COUNT(*) FROM pg_proc WHERE proname = 'pgaadauth_create_principal'"
	readUserSQL   = "SELECT oid::bigint FROM pg_roles WHERE rolname = $1"
)

// newMockedConnector returns a Connector wired to two separate pgxmock pools:
// the target-db pool and the system-db pool used by CreateUser.
// Equality matching is enabled to avoid having to escape SQL for regex.
//
// A cleanup hook asserts that every configured mock expectation was actually
// consumed on both pools — without this, a test that sets up ExpectQuery /
// ExpectExec and forgets to validate would silently pass even if the
// production code stopped issuing the expected SQL.
func newMockedConnector(t *testing.T) (*Connector, pgxmock.PgxPoolIface, pgxmock.PgxPoolIface) {
	t.Helper()
	pool, err := pgxmock.NewPool(pgxmock.QueryMatcherOption(pgxmock.QueryMatcherEqual))
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	sys, err := pgxmock.NewPool(pgxmock.QueryMatcherOption(pgxmock.QueryMatcherEqual))
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(func() {
		if err := pool.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet pgxmock expectations on target pool: %v", err)
		}
		if err := sys.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet pgxmock expectations on sys pool: %v", err)
		}
		pool.Close()
		sys.Close()
	})

	c := &Connector{
		pool:       pool,
		gate:       grantedGate(),
		newSysPool: func() (pgxConn, error) { return sys, nil },
	}
	return c, pool, sys
}

// grantedGate is pre-resolved to full visibility so CRUD tests exercise the
// query under test, not the probe. The probe is covered in preflight_test.go.
func grantedGate() *readAccessGate {
	return &readAccessGate{
		resolved: true,
		access: catalogAccess{
			pgRoles:       true,
			pgAuthMembers: true,
			userName:      "myapp-identity",
		},
	}
}

func expectPgaadAvailable(sys pgxmock.PgxPoolIface) {
	sys.ExpectQuery(checkPgaadSQL).
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(1))
}

func expectPgaadMissing(sys pgxmock.PgxPoolIface) {
	sys.ExpectQuery(checkPgaadSQL).
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(0))
}

func TestCheckPgaadauthExtension_Available(t *testing.T) {
	_, _, sys := newMockedConnector(t)
	expectPgaadAvailable(sys)
	if err := checkPgaadauthExtension(context.Background(), sys); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestCheckPgaadauthExtension_Missing(t *testing.T) {
	_, _, sys := newMockedConnector(t)
	expectPgaadMissing(sys)
	err := checkPgaadauthExtension(context.Background(), sys)
	if err == nil {
		t.Fatalf("expected error when pgaadauth not enabled")
	}
	if !strings.Contains(err.Error(), "Microsoft Entra authentication is not enabled") {
		t.Errorf("expected enable-Entra hint; got %v", err)
	}
}

func TestCheckPgaadauthExtension_QueryError(t *testing.T) {
	_, _, sys := newMockedConnector(t)
	sys.ExpectQuery(checkPgaadSQL).WillReturnError(errors.New("connection refused"))
	err := checkPgaadauthExtension(context.Background(), sys)
	if err == nil {
		t.Fatalf("expected error to propagate")
	}
	if !strings.Contains(err.Error(), "checking for pgaadauth_create_principal") {
		t.Errorf("error should be wrapped; got %v", err)
	}
}

func TestCreateUser_TypeUser(t *testing.T) {
	c, pool, sys := newMockedConnector(t)
	expectPgaadAvailable(sys)
	sys.ExpectExec("SELECT pgaadauth_create_principal($1, false, false)").
		WithArgs("juan.perez@milanesa.com").
		WillReturnResult(pgxmock.NewResult("SELECT", 1))
	pool.ExpectQuery(readUserSQL).
		WithArgs("juan.perez@milanesa.com").
		WillReturnRows(pgxmock.NewRows([]string{"oid"}).AddRow(int64(16384)))

	u := &database.User{Type: "user", Name: "juan.perez@milanesa.com"}
	if err := c.CreateUser(context.Background(), u); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if u.PrincipalID != 16384 {
		t.Errorf("PrincipalID = %d, want 16384", u.PrincipalID)
	}
	if u.DefaultSchema != "public" {
		t.Errorf("DefaultSchema should default to 'public'; got %q", u.DefaultSchema)
	}
}

func TestCreateUser_TypeGroup(t *testing.T) {
	c, pool, sys := newMockedConnector(t)
	expectPgaadAvailable(sys)
	sys.ExpectExec("SELECT pgaadauth_create_principal_with_oid($1, $2, 'group', false, false)").
		WithArgs("db.reader", "00000000-0000-0000-0000-000000000000").
		WillReturnResult(pgxmock.NewResult("SELECT", 1))
	pool.ExpectQuery(readUserSQL).
		WithArgs("db.reader").
		WillReturnRows(pgxmock.NewRows([]string{"oid"}).AddRow(int64(20000)))

	u := &database.User{
		Type: "group", Name: "db.reader", ObjectID: "00000000-0000-0000-0000-000000000000",
	}
	if err := c.CreateUser(context.Background(), u); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestCreateUser_TypeServicePrincipal(t *testing.T) {
	c, pool, sys := newMockedConnector(t)
	expectPgaadAvailable(sys)
	sys.ExpectExec("SELECT pgaadauth_create_principal_with_oid($1, $2, 'service', false, false)").
		WithArgs("myapp-identity", "00000000-0000-0000-0000-000000000000").
		WillReturnResult(pgxmock.NewResult("SELECT", 1))
	pool.ExpectQuery(readUserSQL).
		WithArgs("myapp-identity").
		WillReturnRows(pgxmock.NewRows([]string{"oid"}).AddRow(int64(20001)))

	u := &database.User{
		Type: "service_principal", Name: "myapp-identity", ObjectID: "00000000-0000-0000-0000-000000000000",
	}
	if err := c.CreateUser(context.Background(), u); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestCreateUser_UnknownType(t *testing.T) {
	c, _, sys := newMockedConnector(t)
	expectPgaadAvailable(sys)
	err := c.CreateUser(context.Background(), &database.User{Type: "robot", Name: "x"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "unknown principal type") {
		t.Errorf("unexpected message: %v", err)
	}
}

func TestCreateUser_PgaadauthCallError(t *testing.T) {
	c, _, sys := newMockedConnector(t)
	expectPgaadAvailable(sys)
	sys.ExpectExec("SELECT pgaadauth_create_principal($1, false, false)").
		WithArgs("juan.perez@milanesa.com").
		WillReturnError(errors.New("principal already exists"))

	err := c.CreateUser(context.Background(), &database.User{Type: "user", Name: "juan.perez@milanesa.com"})
	if err == nil {
		t.Fatalf("expected error from pgaadauth call")
	}
	if !strings.Contains(err.Error(), "pgaadauth_create_principal") {
		t.Errorf("error should be wrapped; got %v", err)
	}
}

func TestCreateUser_GroupOidError(t *testing.T) {
	c, _, sys := newMockedConnector(t)
	expectPgaadAvailable(sys)
	sys.ExpectExec("SELECT pgaadauth_create_principal_with_oid($1, $2, 'group', false, false)").
		WithArgs("db.reader", "00000000-0000-0000-0000-000000000000").
		WillReturnError(errors.New("invalid oid"))

	err := c.CreateUser(context.Background(), &database.User{
		Type: "group", Name: "db.reader", ObjectID: "00000000-0000-0000-0000-000000000000",
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "pgaadauth_create_principal_with_oid") {
		t.Errorf("error should be wrapped; got %v", err)
	}
}

func TestCreateUser_ServicePrincipalOidError(t *testing.T) {
	c, _, sys := newMockedConnector(t)
	expectPgaadAvailable(sys)
	sys.ExpectExec("SELECT pgaadauth_create_principal_with_oid($1, $2, 'service', false, false)").
		WithArgs("myapp-identity", "00000000-0000-0000-0000-000000000000").
		WillReturnError(errors.New("not found"))

	err := c.CreateUser(context.Background(), &database.User{
		Type: "service_principal", Name: "myapp-identity", ObjectID: "00000000-0000-0000-0000-000000000000",
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "pgaadauth_create_principal_with_oid") {
		t.Errorf("error should be wrapped; got %v", err)
	}
}

func TestCreateUser_AppliesNonDefaultSchema(t *testing.T) {
	c, pool, sys := newMockedConnector(t)
	expectPgaadAvailable(sys)
	sys.ExpectExec("SELECT pgaadauth_create_principal($1, false, false)").
		WithArgs("juan.perez@milanesa.com").
		WillReturnResult(pgxmock.NewResult("SELECT", 1))
	pool.ExpectExec(`ALTER ROLE "juan.perez@milanesa.com" SET search_path TO "reporting"`).
		WillReturnResult(pgxmock.NewResult("ALTER", 1))
	pool.ExpectQuery(readUserSQL).
		WithArgs("juan.perez@milanesa.com").
		WillReturnRows(pgxmock.NewRows([]string{"oid"}).AddRow(int64(123)))

	u := &database.User{
		Type: "user", Name: "juan.perez@milanesa.com", DefaultSchema: "reporting",
	}
	if err := c.CreateUser(context.Background(), u); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if u.DefaultSchema != "reporting" {
		t.Errorf("DefaultSchema = %q, want reporting", u.DefaultSchema)
	}
}

func TestCreateUser_SchemaPublicSkipped(t *testing.T) {
	c, pool, sys := newMockedConnector(t)
	expectPgaadAvailable(sys)
	sys.ExpectExec("SELECT pgaadauth_create_principal($1, false, false)").
		WithArgs("juan.perez@milanesa.com").
		WillReturnResult(pgxmock.NewResult("SELECT", 1))
	// No ALTER ROLE expected for default schema "public".
	pool.ExpectQuery(readUserSQL).
		WithArgs("juan.perez@milanesa.com").
		WillReturnRows(pgxmock.NewRows([]string{"oid"}).AddRow(int64(123)))

	u := &database.User{
		Type: "user", Name: "juan.perez@milanesa.com", DefaultSchema: "public",
	}
	if err := c.CreateUser(context.Background(), u); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestCreateUser_AlterSchemaError(t *testing.T) {
	c, pool, sys := newMockedConnector(t)
	expectPgaadAvailable(sys)
	sys.ExpectExec("SELECT pgaadauth_create_principal($1, false, false)").
		WithArgs("juan.perez@milanesa.com").
		WillReturnResult(pgxmock.NewResult("SELECT", 1))
	pool.ExpectExec(`ALTER ROLE "juan.perez@milanesa.com" SET search_path TO "reporting"`).
		WillReturnError(errors.New("schema does not exist"))

	err := c.CreateUser(context.Background(), &database.User{
		Type: "user", Name: "juan.perez@milanesa.com", DefaultSchema: "reporting",
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "ALTER ROLE SET search_path") {
		t.Errorf("error should be wrapped; got %v", err)
	}
}

func TestCreateUser_NewSysPoolError(t *testing.T) {
	c, _, _ := newMockedConnector(t)
	c.newSysPool = func() (pgxConn, error) { return nil, errors.New("cannot reach postgres db") }

	err := c.CreateUser(context.Background(), &database.User{Type: "user", Name: "x"})
	if err == nil {
		t.Fatalf("expected sys pool error")
	}
	if !strings.Contains(err.Error(), "opening system database connection") {
		t.Errorf("error should be wrapped; got %v", err)
	}
}

func TestGetUser_Found(t *testing.T) {
	c, pool, _ := newMockedConnector(t)
	pool.ExpectQuery(readUserSQL).
		WithArgs("juan.perez@milanesa.com").
		WillReturnRows(pgxmock.NewRows([]string{"oid"}).AddRow(int64(99)))

	got, err := c.GetUser(context.Background(), "juan.perez@milanesa.com")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got == nil {
		t.Fatalf("expected user")
		return
	}
	if got.PrincipalID != 99 {
		t.Errorf("PrincipalID = %d, want 99", got.PrincipalID)
	}
	if got.DefaultSchema != "public" {
		t.Errorf("DefaultSchema should default to 'public'; got %q", got.DefaultSchema)
	}
}

func TestGetUser_NotFound(t *testing.T) {
	c, pool, _ := newMockedConnector(t)
	pool.ExpectQuery(readUserSQL).
		WithArgs("missing").
		WillReturnError(pgx.ErrNoRows)

	got, err := c.GetUser(context.Background(), "missing")
	if err != nil {
		t.Fatalf("ErrNoRows must be normalised; got %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil; got %+v", got)
	}
}

func TestGetUser_DBError(t *testing.T) {
	c, pool, _ := newMockedConnector(t)
	pool.ExpectQuery(readUserSQL).
		WithArgs("x").
		WillReturnError(errors.New("connection lost"))

	if _, err := c.GetUser(context.Background(), "x"); err == nil {
		t.Fatalf("expected error to propagate")
	}
}

func TestUpdateUser_NonDefaultSchema(t *testing.T) {
	c, pool, _ := newMockedConnector(t)
	pool.ExpectExec(`ALTER ROLE "juan.perez@milanesa.com" SET search_path TO "reporting"`).
		WillReturnResult(pgxmock.NewResult("ALTER", 1))
	pool.ExpectQuery(readUserSQL).
		WithArgs("juan.perez@milanesa.com").
		WillReturnRows(pgxmock.NewRows([]string{"oid"}).AddRow(int64(1)))

	u := &database.User{Name: "juan.perez@milanesa.com", DefaultSchema: "reporting"}
	if err := c.UpdateUser(context.Background(), u); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestUpdateUser_EmptySchemaResetsSearchPath(t *testing.T) {
	c, pool, _ := newMockedConnector(t)
	pool.ExpectExec(`ALTER ROLE "juan.perez@milanesa.com" RESET search_path`).
		WillReturnResult(pgxmock.NewResult("ALTER", 1))
	pool.ExpectQuery(readUserSQL).
		WithArgs("juan.perez@milanesa.com").
		WillReturnRows(pgxmock.NewRows([]string{"oid"}).AddRow(int64(1)))

	u := &database.User{Name: "juan.perez@milanesa.com", DefaultSchema: ""}
	if err := c.UpdateUser(context.Background(), u); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestUpdateUser_AlterError(t *testing.T) {
	c, pool, _ := newMockedConnector(t)
	pool.ExpectExec(`ALTER ROLE "x" SET search_path TO "reporting"`).
		WillReturnError(&pgconn.PgError{Code: "42501", Message: "permission denied"})

	err := c.UpdateUser(context.Background(), &database.User{Name: "x", DefaultSchema: "reporting"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "ALTER ROLE search_path") {
		t.Errorf("error should be wrapped; got %v", err)
	}
}

func TestDeleteUser_Success(t *testing.T) {
	c, pool, _ := newMockedConnector(t)
	pool.ExpectExec(`DROP ROLE IF EXISTS "juan.perez@milanesa.com"`).
		WillReturnResult(pgxmock.NewResult("DROP", 1))
	if err := c.DeleteUser(context.Background(), "juan.perez@milanesa.com"); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestDeleteUser_Error(t *testing.T) {
	c, pool, _ := newMockedConnector(t)
	pool.ExpectExec(`DROP ROLE IF EXISTS "x"`).
		WillReturnError(errors.New("dependent objects exist"))
	if err := c.DeleteUser(context.Background(), "x"); err == nil {
		t.Fatalf("expected error")
	}
}
