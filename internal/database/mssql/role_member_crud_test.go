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

const checkMembershipSQL = `
			SELECT 1
			FROM sys.database_role_members rm
			JOIN sys.database_principals r ON rm.role_principal_id   = r.principal_id
			JOIN sys.database_principals m ON rm.member_principal_id = m.principal_id
			WHERE r.name = @role
			  AND m.name = @member`

func expectMembershipExists(mock sqlmock.Sqlmock, role, member string) {
	mock.ExpectQuery(checkMembershipSQL).
		WithArgs(sql.Named("role", role), sql.Named("member", member)).
		WillReturnRows(sqlmock.NewRows([]string{"col"}).AddRow(1))
}

func expectMembershipAbsent(mock sqlmock.Sqlmock, role, member string) {
	mock.ExpectQuery(checkMembershipSQL).
		WithArgs(sql.Named("role", role), sql.Named("member", member)).
		WillReturnError(sql.ErrNoRows)
}

func TestConnector_GetRoleMember_Exists(t *testing.T) {
	c, mock := newMockedConnector(t)
	expectMembershipExists(mock, "db_datareader", "juan.perez@milanesa.com")

	got, err := c.GetRoleMember(context.Background(), "db_datareader", "juan.perez@milanesa.com")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got == nil {
		t.Fatalf("expected role member to be returned")
		return
	}
	if got.Role != "db_datareader" || got.Member != "juan.perez@milanesa.com" {
		t.Errorf("unexpected fields: %+v", got)
	}
}

func TestConnector_GetRoleMember_Absent(t *testing.T) {
	c, mock := newMockedConnector(t)
	expectMembershipAbsent(mock, "db_datareader", "juan.perez@milanesa.com")

	got, err := c.GetRoleMember(context.Background(), "db_datareader", "juan.perez@milanesa.com")
	if err != nil {
		t.Fatalf("absent membership must not be an error; got %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for absent membership; got %+v", got)
	}
}

func TestConnector_GetRoleMember_DBError(t *testing.T) {
	c, mock := newMockedConnector(t)
	mock.ExpectQuery(checkMembershipSQL).
		WithArgs(sql.Named("role", "r"), sql.Named("member", "m")).
		WillReturnError(errors.New("connection lost"))

	if _, err := c.GetRoleMember(context.Background(), "r", "m"); err == nil {
		t.Fatalf("expected error to propagate")
	}
}

func TestConnector_CreateRoleMember_Success(t *testing.T) {
	c, mock := newMockedConnector(t)
	expectMembershipAbsent(mock, "db_datareader", "juan.perez@milanesa.com")
	mock.ExpectExec(`ALTER ROLE [db_datareader] ADD MEMBER [juan.perez@milanesa.com]`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	rm := &database.RoleMember{Role: "db_datareader", Member: "juan.perez@milanesa.com"}
	if err := c.CreateRoleMember(context.Background(), rm); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestConnector_CreateRoleMember_AlreadyMember_RejectsWithImportHint(t *testing.T) {
	c, mock := newMockedConnector(t)
	expectMembershipExists(mock, "db_datareader", "juan.perez@milanesa.com")

	rm := &database.RoleMember{Role: "db_datareader", Member: "juan.perez@milanesa.com"}
	err := c.CreateRoleMember(context.Background(), rm)
	if err == nil {
		t.Fatalf("expected error when membership already exists")
	}
	if !strings.Contains(err.Error(), "import") {
		t.Errorf("error should suggest import; got %v", err)
	}
}

func TestConnector_CreateRoleMember_GetError(t *testing.T) {
	c, mock := newMockedConnector(t)
	mock.ExpectQuery(checkMembershipSQL).
		WithArgs(sql.Named("role", "r"), sql.Named("member", "m")).
		WillReturnError(errors.New("boom"))

	err := c.CreateRoleMember(context.Background(), &database.RoleMember{Role: "r", Member: "m"})
	if err == nil {
		t.Fatalf("expected GetRoleMember error to surface")
	}
	if !strings.Contains(err.Error(), "checking existing membership") {
		t.Errorf("error should be wrapped; got %v", err)
	}
}

func TestConnector_CreateRoleMember_AlterError(t *testing.T) {
	c, mock := newMockedConnector(t)
	expectMembershipAbsent(mock, "r", "m")
	mock.ExpectExec(`ALTER ROLE [r] ADD MEMBER [m]`).
		WillReturnError(errors.New("permission denied"))

	err := c.CreateRoleMember(context.Background(), &database.RoleMember{Role: "r", Member: "m"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "ALTER ROLE ADD MEMBER") {
		t.Errorf("error should be wrapped; got %v", err)
	}
}

func TestConnector_DeleteRoleMember_Success(t *testing.T) {
	c, mock := newMockedConnector(t)
	expectMembershipExists(mock, "db_datareader", "juan.perez@milanesa.com")
	mock.ExpectExec(`ALTER ROLE [db_datareader] DROP MEMBER [juan.perez@milanesa.com]`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := c.DeleteRoleMember(context.Background(), "db_datareader", "juan.perez@milanesa.com"); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestConnector_DeleteRoleMember_AlreadyAbsent_NoOp(t *testing.T) {
	c, mock := newMockedConnector(t)
	expectMembershipAbsent(mock, "r", "m")

	if err := c.DeleteRoleMember(context.Background(), "r", "m"); err != nil {
		t.Fatalf("absent membership delete should be no-op; got %v", err)
	}
	// No ALTER ROLE DROP MEMBER expected — the cleanup hook on
	// newMockedConnector verifies that no extra SQL was emitted.
}

func TestConnector_DeleteRoleMember_GetError(t *testing.T) {
	c, mock := newMockedConnector(t)
	mock.ExpectQuery(checkMembershipSQL).
		WithArgs(sql.Named("role", "r"), sql.Named("member", "m")).
		WillReturnError(errors.New("boom"))

	err := c.DeleteRoleMember(context.Background(), "r", "m")
	if err == nil {
		t.Fatalf("expected pre-check error to surface")
	}
	if !strings.Contains(err.Error(), "checking membership before delete") {
		t.Errorf("error should be wrapped; got %v", err)
	}
}

func TestConnector_DeleteRoleMember_AlterError(t *testing.T) {
	c, mock := newMockedConnector(t)
	expectMembershipExists(mock, "r", "m")
	mock.ExpectExec(`ALTER ROLE [r] DROP MEMBER [m]`).
		WillReturnError(errors.New("permission denied"))

	err := c.DeleteRoleMember(context.Background(), "r", "m")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "ALTER ROLE DROP MEMBER") {
		t.Errorf("error should be wrapped; got %v", err)
	}
}
