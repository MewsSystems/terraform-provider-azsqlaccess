// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	database "github.com/mews/terraform-provider-azsqlaccess/internal/database"
)

// GetRoleMember checks whether member is already in role.
// Returns nil if the membership does not exist (not an error — just absent).
func (c *Connector) GetRoleMember(ctx context.Context, role, member string) (*database.RoleMember, error) {
	var exists int
	err := c.pool.QueryRow(ctx, `
		SELECT 1
		FROM pg_auth_members am
		JOIN pg_roles r ON am.roleid = r.oid
		JOIN pg_roles m ON am.member  = m.oid
		WHERE r.rolname = $1
		  AND m.rolname = $2`,
		role, member,
	).Scan(&exists)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil // membership absent — not an error
	}
	if err != nil {
		return nil, fmt.Errorf("GetRoleMember: %w", err)
	}
	return &database.RoleMember{Role: role, Member: member}, nil
}

// CreateRoleMember grants member the role using GRANT ... TO ...
// Identifiers are double-quoted; PostgreSQL does not support parameters in DDL.
//
// We pre-check existence because GRANT is idempotent — it succeeds silently
// even if the member already has the role. Without this guard, duplicate
// Terraform resources could silently claim the same DB object, causing state
// corruption on destroy.
func (c *Connector) CreateRoleMember(ctx context.Context, rm *database.RoleMember) error {
	existing, err := c.GetRoleMember(ctx, rm.Role, rm.Member)
	if err != nil {
		return fmt.Errorf("checking existing membership: %w", err)
	}
	if existing != nil {
		return fmt.Errorf("member %q is already in role %q — import this resource instead of creating it", rm.Member, rm.Role)
	}

	stmt := fmt.Sprintf("GRANT %s TO %s", quoteIdentifier(rm.Role), quoteIdentifier(rm.Member))
	if err := withRetry(ctx, func() error {
		_, err := c.pool.Exec(ctx, stmt)
		return err
	}); err != nil {
		return fmt.Errorf("GRANT role: %w", err)
	}
	return nil
}

// DeleteRoleMember revokes the role from member using REVOKE ... FROM ...
// Pre-checks existence so that out-of-band removal (membership already gone)
// is treated as a successful no-op rather than a hard error. Without this,
// a destroy after manual removal would leave Terraform stuck.
func (c *Connector) DeleteRoleMember(ctx context.Context, role, member string) error {
	existing, err := c.GetRoleMember(ctx, role, member)
	if err != nil {
		return fmt.Errorf("checking membership before delete: %w", err)
	}
	if existing == nil {
		return nil // already gone — nothing to do
	}

	stmt := fmt.Sprintf("REVOKE %s FROM %s", quoteIdentifier(role), quoteIdentifier(member))
	if err := withRetry(ctx, func() error {
		_, err := c.pool.Exec(ctx, stmt)
		return err
	}); err != nil {
		return fmt.Errorf("REVOKE role: %w", err)
	}
	return nil
}
