// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mews/terraform-provider-azsqlaccess/internal/database"
	"github.com/pashagolub/pgxmock/v4"
)

func expectProbe(pool pgxmock.PgxPoolIface, pgRoles, pgAuthMembers bool) {
	pool.ExpectQuery(probeCatalogAccessSQL).
		WillReturnRows(
			pgxmock.NewRows([]string{"pg_roles", "pg_auth_members", "current_user"}).
				AddRow(pgRoles, pgAuthMembers, "myapp-identity"),
		)
}

// newUngatedConnector is newMockedConnector without the pre-resolved gate, so
// the probe itself runs.
func newUngatedConnector(t *testing.T) (*Connector, pgxmock.PgxPoolIface) {
	t.Helper()
	c, pool, _ := newMockedConnector(t)
	c.gate = &readAccessGate{}
	return c, pool
}

// The default PostgreSQL grants make this the case everyone actually hits.
func TestCheckReadAccess_DefaultGrants_AllowBothScopes(t *testing.T) {
	c, pool := newUngatedConnector(t)
	expectProbe(pool, true, true)

	if err := c.CheckReadAccess(context.Background(), database.ReadScopeUser); err != nil {
		t.Fatalf("user scope: unexpected error: %v", err)
	}
	// Second call must not re-probe — pgxmock would fail on an unexpected query.
	if err := c.CheckReadAccess(context.Background(), database.ReadScopeRoleMember); err != nil {
		t.Fatalf("role_member scope: unexpected error: %v", err)
	}
}

// pg_auth_members is publicly readable by default, but that default is
// revocable, which is the reason this engine is probed at all rather than
// assumed to be fine.
func TestCheckReadAccess_AuthMembersRevoked_AllowsUsersRefusesRoleMembers(t *testing.T) {
	c, pool := newUngatedConnector(t)
	expectProbe(pool, true, false)

	if err := c.CheckReadAccess(context.Background(), database.ReadScopeUser); err != nil {
		t.Fatalf("user scope: unexpected error: %v", err)
	}

	err := c.CheckReadAccess(context.Background(), database.ReadScopeRoleMember)
	if err == nil {
		t.Fatal("role_member scope: expected an error, got nil")
	}
	for _, want := range []string{"pg_catalog.pg_auth_members", "myapp-identity"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestCheckReadAccess_ProbeFailure_IsNotMemoised(t *testing.T) {
	c, pool := newUngatedConnector(t)

	transportErr := errors.New("connection reset by peer")
	pool.ExpectQuery(probeCatalogAccessSQL).WillReturnError(transportErr)

	if err := c.CheckReadAccess(context.Background(), database.ReadScopeUser); !errors.Is(err, transportErr) {
		t.Fatalf("first call: got %v, want it to wrap %v", err, transportErr)
	}

	expectProbe(pool, true, true)
	if err := c.CheckReadAccess(context.Background(), database.ReadScopeUser); err != nil {
		t.Fatalf("second call: unexpected error: %v", err)
	}
}

func TestGetUser_WithoutVisibility_ErrorsRatherThanReportingAbsent(t *testing.T) {
	c, pool := newUngatedConnector(t)
	expectProbe(pool, false, false)

	user, err := c.GetUser(context.Background(), "juan.perez@milanesa.com")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if user != nil {
		t.Errorf("expected no user alongside the error, got %+v", user)
	}
	if !strings.Contains(err.Error(), "pg_catalog.pg_roles") {
		t.Errorf("error %q does not name the catalog", err)
	}
}

func TestGetRoleMember_WithoutVisibility_ErrorsRatherThanReportingAbsent(t *testing.T) {
	c, pool := newUngatedConnector(t)
	expectProbe(pool, false, false)

	rm, err := c.GetRoleMember(context.Background(), "pg_read_all_data", "juan.perez@milanesa.com")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if rm != nil {
		t.Errorf("expected no membership alongside the error, got %+v", rm)
	}
}
