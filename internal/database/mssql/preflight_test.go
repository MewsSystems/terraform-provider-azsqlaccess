// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package mssql

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/mews/terraform-provider-azsqlaccess/internal/database"
)

// expectProbe queues the permission probe with the given effective permissions.
// A nil value stands for the NULL that HAS_PERMS_BY_NAME returns when it does
// not recognise the permission name.
func expectProbe(mock sqlmock.Sqlmock, viewDefinition, alterAnyUser any) {
	mock.ExpectQuery(probeCatalogAccessSQL).
		WillReturnRows(
			sqlmock.NewRows([]string{"view_definition", "alter_any_user", "user_name", "db_name"}).
				AddRow(viewDefinition, alterAnyUser, "myapp-identity", "mydb"),
		)
}

// newUngatedConnector is newMockedConnector without the pre-resolved gate, so
// the probe itself runs.
func newUngatedConnector(t *testing.T) (*Connector, sqlmock.Sqlmock) {
	t.Helper()
	c, mock := newMockedConnector(t)
	c.gate = &readAccessGate{}
	return c, mock
}

func TestCheckReadAccess_ViewDefinition_AllowsBothScopes(t *testing.T) {
	c, mock := newUngatedConnector(t)
	expectProbe(mock, 1, 0)

	if err := c.CheckReadAccess(context.Background(), database.ReadScopeUser); err != nil {
		t.Fatalf("user scope: unexpected error: %v", err)
	}
	// Second call must not re-probe — sqlmock would fail on an unexpected query.
	if err := c.CheckReadAccess(context.Background(), database.ReadScopeRoleMember); err != nil {
		t.Fatalf("role_member scope: unexpected error: %v", err)
	}
}

// db_accessadmin holds ALTER ANY USER but not VIEW DEFINITION: it can see other
// users and cannot see their role memberships. The provider must allow the one
// and refuse the other rather than picking a single blanket requirement.
func TestCheckReadAccess_AlterAnyUserOnly_AllowsUsersRefusesRoleMembers(t *testing.T) {
	c, mock := newUngatedConnector(t)
	expectProbe(mock, 0, 1)

	if err := c.CheckReadAccess(context.Background(), database.ReadScopeUser); err != nil {
		t.Fatalf("user scope: unexpected error: %v", err)
	}

	err := c.CheckReadAccess(context.Background(), database.ReadScopeRoleMember)
	if err == nil {
		t.Fatal("role_member scope: expected an error, got nil")
	}
	for _, want := range []string{"sys.database_role_members", "VIEW DEFINITION", "myapp-identity"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestCheckReadAccess_NoPermissions_RefusesBothScopes(t *testing.T) {
	c, mock := newUngatedConnector(t)
	expectProbe(mock, 0, 0)

	for _, scope := range []database.ReadScope{database.ReadScopeUser, database.ReadScopeRoleMember} {
		if err := c.CheckReadAccess(context.Background(), scope); err == nil {
			t.Errorf("scope %q: expected an error, got nil", scope)
		}
	}
}

// HAS_PERMS_BY_NAME returns NULL for an unrecognised permission name. Reading
// that as "allowed" would reintroduce exactly the silent blindness this guards
// against, so NULL must count as not proven.
func TestCheckReadAccess_NullProbeResult_TreatedAsDenied(t *testing.T) {
	c, mock := newUngatedConnector(t)
	expectProbe(mock, nil, nil)

	if err := c.CheckReadAccess(context.Background(), database.ReadScopeUser); err == nil {
		t.Fatal("expected an error for a NULL probe result, got nil")
	}
}

// A probe that could not run at all says nothing about permissions, so it must
// not be memoised as a verdict — the next operation gets a fresh attempt.
func TestCheckReadAccess_ProbeFailure_IsNotMemoised(t *testing.T) {
	c, mock := newUngatedConnector(t)

	transportErr := errors.New("connection reset by peer")
	mock.ExpectQuery(probeCatalogAccessSQL).WillReturnError(transportErr)

	if err := c.CheckReadAccess(context.Background(), database.ReadScopeUser); !errors.Is(err, transportErr) {
		t.Fatalf("first call: got %v, want it to wrap %v", err, transportErr)
	}

	expectProbe(mock, 1, 1)
	if err := c.CheckReadAccess(context.Background(), database.ReadScopeUser); err != nil {
		t.Fatalf("second call: unexpected error: %v", err)
	}
}

// The whole point of the gate: a blind identity must never let a read report
// "absent", because the caller turns that into RemoveResource.
func TestGetUser_WithoutVisibility_ErrorsRatherThanReportingAbsent(t *testing.T) {
	c, mock := newUngatedConnector(t)
	expectProbe(mock, 0, 0)

	user, err := c.GetUser(context.Background(), "myapp-identity")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if user != nil {
		t.Errorf("expected no user alongside the error, got %+v", user)
	}
	if !strings.Contains(err.Error(), "sys.database_principals") {
		t.Errorf("error %q does not name the catalog view", err)
	}
}

func TestGetRoleMember_WithoutVisibility_ErrorsRatherThanReportingAbsent(t *testing.T) {
	c, mock := newUngatedConnector(t)
	expectProbe(mock, 0, 0)

	rm, err := c.GetRoleMember(context.Background(), "db_owner", "myapp-identity")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if rm != nil {
		t.Errorf("expected no membership alongside the error, got %+v", rm)
	}
}
