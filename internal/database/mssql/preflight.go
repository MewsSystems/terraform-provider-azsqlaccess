// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package mssql

import (
	"context"
	"database/sql"
	"fmt"
	"sync"

	database_pkg "github.com/mews/terraform-provider-azsqlaccess/internal/database"
)

// HAS_PERMS_BY_NAME evaluates effective permissions, so db_owner, dbo and the
// Entra administrator all answer 1 without being special-cased.
// USER_NAME() and DB_NAME() ride along so the failure can name what to fix.
const probeCatalogAccessSQL = `
	SELECT
		HAS_PERMS_BY_NAME(DB_NAME(), 'DATABASE', 'VIEW DEFINITION'),
		HAS_PERMS_BY_NAME(DB_NAME(), 'DATABASE', 'ALTER ANY USER'),
		USER_NAME(),
		DB_NAME()`

// catalogAccess is what the identity may see. The two permissions are not
// interchangeable: ALTER ANY USER reveals sys.database_principals but not
// sys.database_role_members, which needs VIEW DEFINITION. db_accessadmin lands
// exactly there.
type catalogAccess struct {
	viewDefinition bool
	alterAnyUser   bool
	userName       string
	databaseName   string
}

// readAccessGate memoises the probe for one (server, database), so a stack with
// twenty users runs one probe rather than twenty.
type readAccessGate struct {
	mu       sync.Mutex
	resolved bool
	access   catalogAccess
}

// resolve probes on first call. Only a definitive answer is memoised: a probe
// that could not run says nothing about permissions, so it is retried rather
// than poisoning the run. The lock is held across the round trip so concurrent
// first-callers wait for one probe instead of each issuing their own.
func (g *readAccessGate) resolve(ctx context.Context, db *sql.DB) (catalogAccess, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.resolved {
		return g.access, nil
	}

	var viewDefinition, alterAnyUser sql.NullInt64
	var userName, databaseName string
	err := db.QueryRowContext(ctx, probeCatalogAccessSQL).
		Scan(&viewDefinition, &alterAnyUser, &userName, &databaseName)
	if err != nil {
		return catalogAccess{}, fmt.Errorf("checking catalog read permissions: %w", err)
	}

	// NULL means the server did not recognise the permission name. Never access.
	g.access = catalogAccess{
		viewDefinition: viewDefinition.Valid && viewDefinition.Int64 == 1,
		alterAnyUser:   alterAnyUser.Valid && alterAnyUser.Int64 == 1,
		userName:       userName,
		databaseName:   databaseName,
	}
	g.resolved = true
	return g.access, nil
}

func (c *Connector) CheckReadAccess(ctx context.Context, scope database_pkg.ReadScope) error {
	access, err := c.gate.resolve(ctx, c.db)
	if err != nil {
		return err
	}

	switch scope {
	case database_pkg.ReadScopeUser:
		if access.viewDefinition || access.alterAnyUser {
			return nil
		}
		return missingAccessError(access, "sys.database_principals", "users", "VIEW DEFINITION or ALTER ANY USER")

	case database_pkg.ReadScopeRoleMember:
		if access.viewDefinition {
			return nil
		}
		return missingAccessError(access, "sys.database_role_members", "role memberships", "VIEW DEFINITION")

	default:
		return fmt.Errorf("unknown read scope %q", scope)
	}
}

func missingAccessError(access catalogAccess, view, subject, needed string) error {
	// Multi-line operator-facing diagnostic; staticcheck's ST1005 does not fit
	// a help text.
	return fmt.Errorf( //nolint:staticcheck
		"database user %q cannot see other %s in %s on database %q\n\n"+
			"Azure SQL limits catalog metadata to principals the caller owns or holds a "+
			"permission on, and it does so silently: an unauthorised read returns fewer "+
			"rows rather than an error. Left unchecked, every %s managed by this provider "+
			"would read back as absent and Terraform would propose to recreate resources "+
			"that already exist.\n\n"+
			"Grant the identity running Terraform %s on the database, for example:\n"+
			"  GRANT VIEW DEFINITION TO %s;\n\n"+
			"Membership of db_securityadmin also carries VIEW DEFINITION, but it brings "+
			"ALTER ANY ROLE with it and Microsoft flags it as a privilege-elevation risk",
		access.userName, subject, view, access.databaseName, subject, needed, quoteName(access.userName),
	)
}
