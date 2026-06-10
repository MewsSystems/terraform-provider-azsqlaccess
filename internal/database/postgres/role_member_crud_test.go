// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/mews/terraform-provider-azsqlaccess/internal/database"
	"github.com/pashagolub/pgxmock/v4"
)

const checkMembershipSQL = `
			SELECT 1
			FROM pg_auth_members am
			JOIN pg_roles r ON am.roleid = r.oid
			JOIN pg_roles m ON am.member  = m.oid
			WHERE r.rolname = $1
			  AND m.rolname = $2`

func expectMembershipExists(pool pgxmock.PgxPoolIface, role, member string) {
	pool.ExpectQuery(checkMembershipSQL).
		WithArgs(role, member).
		WillReturnRows(pgxmock.NewRows([]string{"col"}).AddRow(1))
}

func expectMembershipAbsent(pool pgxmock.PgxPoolIface, role, member string) {
	pool.ExpectQuery(checkMembershipSQL).
		WithArgs(role, member).
		WillReturnError(pgx.ErrNoRows)
}

func TestGetRoleMember_Exists(t *testing.T) {
	c, pool, _ := newMockedConnector(t)
	expectMembershipExists(pool, "pg_read_all_data", "juan.perez@milanesa.com")

	got, err := c.GetRoleMember(context.Background(), "pg_read_all_data", "juan.perez@milanesa.com")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got == nil {
		t.Fatalf("expected role member")
		return
	}
	if got.Role != "pg_read_all_data" || got.Member != "juan.perez@milanesa.com" {
		t.Errorf("unexpected fields: %+v", got)
	}
}

func TestGetRoleMember_Absent(t *testing.T) {
	c, pool, _ := newMockedConnector(t)
	expectMembershipAbsent(pool, "pg_read_all_data", "juan.perez@milanesa.com")

	got, err := c.GetRoleMember(context.Background(), "pg_read_all_data", "juan.perez@milanesa.com")
	if err != nil {
		t.Fatalf("absent membership must not error; got %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil; got %+v", got)
	}
}

func TestGetRoleMember_DBError(t *testing.T) {
	c, pool, _ := newMockedConnector(t)
	pool.ExpectQuery(checkMembershipSQL).
		WithArgs("r", "m").
		WillReturnError(errors.New("connection lost"))

	if _, err := c.GetRoleMember(context.Background(), "r", "m"); err == nil {
		t.Fatalf("expected propagated error")
	}
}

func TestCreateRoleMember_Success(t *testing.T) {
	c, pool, _ := newMockedConnector(t)
	expectMembershipAbsent(pool, "pg_read_all_data", "juan.perez@milanesa.com")
	pool.ExpectExec(`GRANT "pg_read_all_data" TO "juan.perez@milanesa.com"`).
		WillReturnResult(pgxmock.NewResult("GRANT", 1))

	rm := &database.RoleMember{Role: "pg_read_all_data", Member: "juan.perez@milanesa.com"}
	if err := c.CreateRoleMember(context.Background(), rm); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestCreateRoleMember_AlreadyMember(t *testing.T) {
	c, pool, _ := newMockedConnector(t)
	expectMembershipExists(pool, "pg_read_all_data", "juan.perez@milanesa.com")

	err := c.CreateRoleMember(context.Background(), &database.RoleMember{
		Role: "pg_read_all_data", Member: "juan.perez@milanesa.com",
	})
	if err == nil {
		t.Fatalf("expected error when membership exists")
	}
	if !strings.Contains(err.Error(), "import this resource") {
		t.Errorf("error should suggest import; got %v", err)
	}
}

func TestCreateRoleMember_GetError(t *testing.T) {
	c, pool, _ := newMockedConnector(t)
	pool.ExpectQuery(checkMembershipSQL).
		WithArgs("r", "m").
		WillReturnError(errors.New("boom"))

	err := c.CreateRoleMember(context.Background(), &database.RoleMember{Role: "r", Member: "m"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "checking existing membership") {
		t.Errorf("error should be wrapped; got %v", err)
	}
}

func TestCreateRoleMember_GrantError(t *testing.T) {
	c, pool, _ := newMockedConnector(t)
	expectMembershipAbsent(pool, "r", "m")
	pool.ExpectExec(`GRANT "r" TO "m"`).WillReturnError(errors.New("permission denied"))

	err := c.CreateRoleMember(context.Background(), &database.RoleMember{Role: "r", Member: "m"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "GRANT role") {
		t.Errorf("error should be wrapped; got %v", err)
	}
}

func TestDeleteRoleMember_Success(t *testing.T) {
	c, pool, _ := newMockedConnector(t)
	expectMembershipExists(pool, "pg_read_all_data", "juan.perez@milanesa.com")
	pool.ExpectExec(`REVOKE "pg_read_all_data" FROM "juan.perez@milanesa.com"`).
		WillReturnResult(pgxmock.NewResult("REVOKE", 1))

	if err := c.DeleteRoleMember(context.Background(), "pg_read_all_data", "juan.perez@milanesa.com"); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestDeleteRoleMember_AlreadyAbsent_NoOp(t *testing.T) {
	c, pool, _ := newMockedConnector(t)
	expectMembershipAbsent(pool, "r", "m")

	if err := c.DeleteRoleMember(context.Background(), "r", "m"); err != nil {
		t.Fatalf("absent membership delete must be no-op; got %v", err)
	}
}

func TestDeleteRoleMember_GetError(t *testing.T) {
	c, pool, _ := newMockedConnector(t)
	pool.ExpectQuery(checkMembershipSQL).
		WithArgs("r", "m").
		WillReturnError(errors.New("boom"))

	err := c.DeleteRoleMember(context.Background(), "r", "m")
	if err == nil {
		t.Fatalf("expected propagated error")
	}
	if !strings.Contains(err.Error(), "checking membership before delete") {
		t.Errorf("error should be wrapped; got %v", err)
	}
}

func TestDeleteRoleMember_RevokeError(t *testing.T) {
	c, pool, _ := newMockedConnector(t)
	expectMembershipExists(pool, "r", "m")
	pool.ExpectExec(`REVOKE "r" FROM "m"`).WillReturnError(errors.New("permission denied"))

	err := c.DeleteRoleMember(context.Background(), "r", "m")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "REVOKE role") {
		t.Errorf("error should be wrapped; got %v", err)
	}
}
