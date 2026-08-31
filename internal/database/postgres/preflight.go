// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package postgres

import (
	"context"
	"fmt"
	"sync"

	database_pkg "github.com/mews/terraform-provider-azsqlaccess/internal/database"
)

// PostgreSQL masks catalog columns, not rows, and both catalogs are readable by
// PUBLIC out of the box — so this almost always passes. It is probed rather than
// assumed because that default is revocable.
const probeCatalogAccessSQL = `
	SELECT
		has_table_privilege(current_user, 'pg_catalog.pg_roles', 'SELECT'),
		has_table_privilege(current_user, 'pg_catalog.pg_auth_members', 'SELECT'),
		current_user`

// catalogAccess is what the connected role may read. Membership joins
// pg_auth_members against pg_roles, so it needs both.
type catalogAccess struct {
	pgRoles       bool
	pgAuthMembers bool
	userName      string
}

// readAccessGate memoises the probe for one (server, database), so a run pays
// for it once per database rather than once per resource Read.
type readAccessGate struct {
	mu       sync.Mutex
	resolved bool
	access   catalogAccess
}

// resolve probes on first call. Only a definitive answer is memoised: a probe
// that could not run says nothing about permissions, so it is retried rather
// than poisoning the run.
func (g *readAccessGate) resolve(ctx context.Context, pool pgxConn) (catalogAccess, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.resolved {
		return g.access, nil
	}

	var access catalogAccess
	err := pool.QueryRow(ctx, probeCatalogAccessSQL).
		Scan(&access.pgRoles, &access.pgAuthMembers, &access.userName)
	if err != nil {
		return catalogAccess{}, fmt.Errorf("checking catalog read permissions: %w", err)
	}

	g.access = access
	g.resolved = true
	return g.access, nil
}

func (c *Connector) CheckReadAccess(ctx context.Context, scope database_pkg.ReadScope) error {
	access, err := c.gate.resolve(ctx, c.pool)
	if err != nil {
		return err
	}

	switch scope {
	case database_pkg.ReadScopeUser:
		if access.pgRoles {
			return nil
		}
		return missingAccessError(access.userName, "users", "pg_catalog.pg_roles")

	case database_pkg.ReadScopeRoleMember:
		if access.pgRoles && access.pgAuthMembers {
			return nil
		}
		missing := "pg_catalog.pg_auth_members"
		if !access.pgRoles {
			missing = "pg_catalog.pg_roles and pg_catalog.pg_auth_members"
		}
		return missingAccessError(access.userName, "role memberships", missing)

	default:
		return fmt.Errorf("unknown read scope %q", scope)
	}
}

func missingAccessError(userName, subject, catalog string) error {
	// Multi-line operator-facing diagnostic; staticcheck's ST1005 does not fit
	// a help text.
	return fmt.Errorf( //nolint:staticcheck
		"role %q cannot read %s, so %s managed by this provider cannot be verified\n\n"+
			"Without it every %s would read back as absent and Terraform would propose to "+
			"recreate resources that already exist.\n\n"+
			"Restore the default, which PostgreSQL grants out of the box:\n"+
			"  GRANT SELECT ON %s TO PUBLIC;",
		userName, catalog, subject, subject, catalog,
	)
}
