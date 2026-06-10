// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package mssql

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/mews/terraform-provider-azsqlaccess/internal/database"
)

// newMockedConnector returns a Connector wired to a sqlmock-backed *sql.DB.
// Equality matching is enabled so test queries can be matched verbatim
// (regex matching is the default and would force escaping every SQL token).
//
// A cleanup hook asserts that every configured mock expectation was actually
// consumed — without this, a test that sets up ExpectQuery/ExpectExec and
// forgets to validate would silently pass even if the production code stopped
// issuing the expected SQL.
func newMockedConnector(t *testing.T) (*Connector, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet sqlmock expectations: %v", err)
		}
		_ = db.Close()
	})
	return &Connector{db: db}, mock
}

const readUserSQL = `
			SELECT
				principal_id,
				COALESCE(default_schema_name, '')
			FROM sys.database_principals
			WHERE name = @name
			  AND type IN ('E', 'X')`

func TestConnector_CreateUser_TypeUser(t *testing.T) {
	c, mock := newMockedConnector(t)

	mock.ExpectExec(`CREATE USER [juan.perez@milanesa.com] FROM EXTERNAL PROVIDER`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(readUserSQL).
		WithArgs(sql.Named("name", "juan.perez@milanesa.com")).
		WillReturnRows(sqlmock.NewRows([]string{"principal_id", "default_schema_name"}).AddRow(7, "dbo"))

	u := &database.User{Type: "user", Name: "juan.perez@milanesa.com"}
	if err := c.CreateUser(context.Background(), u); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.PrincipalID != 7 {
		t.Errorf("PrincipalID = %d, want 7", u.PrincipalID)
	}
	if u.DefaultSchema != "dbo" {
		t.Errorf("DefaultSchema = %q, want dbo", u.DefaultSchema)
	}
}

func TestConnector_CreateUser_TypeGroup_WithObjectID(t *testing.T) {
	c, mock := newMockedConnector(t)

	mock.ExpectExec(`CREATE USER [db.reader] FROM EXTERNAL PROVIDER WITH OBJECT_ID = '00000000-0000-0000-0000-000000000000'`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(readUserSQL).
		WithArgs(sql.Named("name", "db.reader")).
		WillReturnRows(sqlmock.NewRows([]string{"principal_id", "default_schema_name"}).AddRow(11, ""))

	u := &database.User{
		Type:     "group",
		Name:     "db.reader",
		ObjectID: "00000000-0000-0000-0000-000000000000",
	}
	if err := c.CreateUser(context.Background(), u); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.PrincipalID != 11 {
		t.Errorf("PrincipalID = %d, want 11", u.PrincipalID)
	}
}

func TestConnector_CreateUser_TypeServicePrincipal(t *testing.T) {
	c, mock := newMockedConnector(t)

	mock.ExpectExec(`CREATE USER [myapp-identity] FROM EXTERNAL PROVIDER WITH OBJECT_ID = '00000000-0000-0000-0000-000000000000'`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(readUserSQL).
		WithArgs(sql.Named("name", "myapp-identity")).
		WillReturnRows(sqlmock.NewRows([]string{"principal_id", "default_schema_name"}).AddRow(12, ""))

	u := &database.User{
		Type:     "service_principal",
		Name:     "myapp-identity",
		ObjectID: "00000000-0000-0000-0000-000000000000",
	}
	if err := c.CreateUser(context.Background(), u); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConnector_CreateUser_GroupRequiresValidUUID(t *testing.T) {
	c, _ := newMockedConnector(t)
	u := &database.User{Type: "group", Name: "db.reader", ObjectID: "not-a-uuid"}
	err := c.CreateUser(context.Background(), u)
	if err == nil {
		t.Fatalf("expected error for invalid UUID")
	}
	if !strings.Contains(err.Error(), "not a valid UUID") {
		t.Errorf("unexpected message: %v", err)
	}
}

func TestConnector_CreateUser_UnknownType(t *testing.T) {
	c, _ := newMockedConnector(t)
	u := &database.User{Type: "robot", Name: "x"}
	err := c.CreateUser(context.Background(), u)
	if err == nil {
		t.Fatalf("expected error for unknown type")
	}
	if !strings.Contains(err.Error(), "unknown principal type") {
		t.Errorf("unexpected message: %v", err)
	}
}

func TestConnector_CreateUser_AppliesNonDefaultSchema(t *testing.T) {
	c, mock := newMockedConnector(t)

	mock.ExpectExec(`CREATE USER [juan.perez@milanesa.com] FROM EXTERNAL PROVIDER`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`ALTER USER [juan.perez@milanesa.com] WITH DEFAULT_SCHEMA = [reporting]`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(readUserSQL).
		WithArgs(sql.Named("name", "juan.perez@milanesa.com")).
		WillReturnRows(sqlmock.NewRows([]string{"principal_id", "default_schema_name"}).AddRow(1, "reporting"))

	u := &database.User{
		Type:          "user",
		Name:          "juan.perez@milanesa.com",
		DefaultSchema: "reporting",
	}
	if err := c.CreateUser(context.Background(), u); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConnector_CreateUser_DefaultSchemaSkippedForDbo(t *testing.T) {
	// Schema "dbo" is the engine default — no ALTER should be emitted.
	c, mock := newMockedConnector(t)

	mock.ExpectExec(`CREATE USER [juan.perez@milanesa.com] FROM EXTERNAL PROVIDER`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(readUserSQL).
		WithArgs(sql.Named("name", "juan.perez@milanesa.com")).
		WillReturnRows(sqlmock.NewRows([]string{"principal_id", "default_schema_name"}).AddRow(1, "dbo"))

	u := &database.User{Type: "user", Name: "juan.perez@milanesa.com", DefaultSchema: "dbo"}
	if err := c.CreateUser(context.Background(), u); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConnector_CreateUser_AlterSchemaError(t *testing.T) {
	c, mock := newMockedConnector(t)

	mock.ExpectExec(`CREATE USER [juan.perez@milanesa.com] FROM EXTERNAL PROVIDER`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`ALTER USER [juan.perez@milanesa.com] WITH DEFAULT_SCHEMA = [reporting]`).
		WillReturnError(errors.New("schema does not exist"))

	u := &database.User{Type: "user", Name: "juan.perez@milanesa.com", DefaultSchema: "reporting"}
	err := c.CreateUser(context.Background(), u)
	if err == nil {
		t.Fatalf("expected error from ALTER USER failure")
	}
	if !strings.Contains(err.Error(), "ALTER USER DEFAULT_SCHEMA") {
		t.Errorf("error should be wrapped with operation context; got %v", err)
	}
}

func TestConnector_CreateUser_TranslatesAlreadyExists(t *testing.T) {
	c, mock := newMockedConnector(t)

	mock.ExpectExec(`CREATE USER [juan.perez@milanesa.com] FROM EXTERNAL PROVIDER`).
		WillReturnError(errors.New("User or role 'juan.perez@milanesa.com' already exists in the current database."))

	u := &database.User{Type: "user", Name: "juan.perez@milanesa.com"}
	err := c.CreateUser(context.Background(), u)
	if err == nil {
		t.Fatalf("expected wrapped error")
	}
	if !strings.Contains(err.Error(), "terraform import") {
		t.Errorf("translateCreateUserError should suggest import; got %v", err)
	}
}

func TestConnector_GetUser_Found(t *testing.T) {
	c, mock := newMockedConnector(t)

	mock.ExpectQuery(readUserSQL).
		WithArgs(sql.Named("name", "juan.perez@milanesa.com")).
		WillReturnRows(sqlmock.NewRows([]string{"principal_id", "default_schema_name"}).AddRow(99, "dbo"))

	got, err := c.GetUser(context.Background(), "juan.perez@milanesa.com")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got == nil {
		t.Fatalf("expected user, got nil")
		return
	}
	if got.PrincipalID != 99 || got.DefaultSchema != "dbo" {
		t.Errorf("unexpected fields: %+v", got)
	}
}

func TestConnector_GetUser_NotFound(t *testing.T) {
	c, mock := newMockedConnector(t)

	mock.ExpectQuery(readUserSQL).
		WithArgs(sql.Named("name", "missing")).
		WillReturnError(sql.ErrNoRows)

	got, err := c.GetUser(context.Background(), "missing")
	if err != nil {
		t.Fatalf("ErrNoRows should be normalised to (nil, nil); got err=%v", err)
	}
	if got != nil {
		t.Fatalf("expected nil user, got %+v", got)
	}
}

func TestConnector_GetUser_DBError(t *testing.T) {
	c, mock := newMockedConnector(t)

	mock.ExpectQuery(readUserSQL).
		WithArgs(sql.Named("name", "x")).
		WillReturnError(errors.New("connection lost"))

	if _, err := c.GetUser(context.Background(), "x"); err == nil {
		t.Fatalf("expected error to propagate")
	}
}

func TestConnector_UpdateUser_NonDefaultSchema(t *testing.T) {
	c, mock := newMockedConnector(t)

	mock.ExpectExec(`ALTER USER [juan.perez@milanesa.com] WITH DEFAULT_SCHEMA = [reporting]`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(readUserSQL).
		WithArgs(sql.Named("name", "juan.perez@milanesa.com")).
		WillReturnRows(sqlmock.NewRows([]string{"principal_id", "default_schema_name"}).AddRow(1, "reporting"))

	u := &database.User{Type: "user", Name: "juan.perez@milanesa.com", DefaultSchema: "reporting"}
	if err := c.UpdateUser(context.Background(), u); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestConnector_UpdateUser_EmptySchemaFallsBackToDbo(t *testing.T) {
	c, mock := newMockedConnector(t)

	mock.ExpectExec(`ALTER USER [juan.perez@milanesa.com] WITH DEFAULT_SCHEMA = [dbo]`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(readUserSQL).
		WithArgs(sql.Named("name", "juan.perez@milanesa.com")).
		WillReturnRows(sqlmock.NewRows([]string{"principal_id", "default_schema_name"}).AddRow(1, "dbo"))

	u := &database.User{Type: "user", Name: "juan.perez@milanesa.com", DefaultSchema: ""}
	if err := c.UpdateUser(context.Background(), u); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestConnector_UpdateUser_AlterError(t *testing.T) {
	c, mock := newMockedConnector(t)
	mock.ExpectExec(`ALTER USER [x] WITH DEFAULT_SCHEMA = [reporting]`).
		WillReturnError(errors.New("permission denied"))

	err := c.UpdateUser(context.Background(), &database.User{Name: "x", DefaultSchema: "reporting"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "ALTER USER DEFAULT_SCHEMA") {
		t.Errorf("error should be wrapped; got %v", err)
	}
}

func TestConnector_DeleteUser_Success(t *testing.T) {
	c, mock := newMockedConnector(t)
	mock.ExpectExec(`DROP USER IF EXISTS [juan.perez@milanesa.com]`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := c.DeleteUser(context.Background(), "juan.perez@milanesa.com"); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestConnector_DeleteUser_Error(t *testing.T) {
	c, mock := newMockedConnector(t)
	mock.ExpectExec(`DROP USER IF EXISTS [x]`).
		WillReturnError(errors.New("dependent objects exist"))
	if err := c.DeleteUser(context.Background(), "x"); err == nil {
		t.Fatalf("expected error")
	}
}
