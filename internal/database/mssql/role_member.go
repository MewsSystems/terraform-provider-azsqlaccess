// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package mssql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	database_pkg "github.com/mews/terraform-provider-azsqlaccess/internal/database"
)

// GetRoleMember checks whether member is already in role.
// Returns nil if the membership does not exist (not an error — just absent).
//
// That absence is only trustworthy once visibility is proven, hence the gate.
// Anything privileged enough to run ALTER ROLE ADD MEMBER already holds VIEW
// DEFINITION, so gating the pre-checks in Create/DeleteRoleMember costs nothing.
func (c *Connector) GetRoleMember(ctx context.Context, role, member string) (*database_pkg.RoleMember, error) {
	if err := c.CheckReadAccess(ctx, database_pkg.ReadScopeRoleMember); err != nil {
		return nil, err
	}

	const q = `
		SELECT 1
		FROM sys.database_role_members rm
		JOIN sys.database_principals r ON rm.role_principal_id   = r.principal_id
		JOIN sys.database_principals m ON rm.member_principal_id = m.principal_id
		WHERE r.name = @role
		  AND m.name = @member`

	var exists int
	err := c.db.QueryRowContext(ctx, q,
		sql.Named("role", role),
		sql.Named("member", member),
	).Scan(&exists)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil // membership absent — not an error
	}
	if err != nil {
		return nil, fmt.Errorf("GetRoleMember: %w", err)
	}
	return &database_pkg.RoleMember{Role: role, Member: member}, nil
}

// CreateRoleMember adds member to role using ALTER ROLE ... ADD MEMBER.
// Identifiers are bracket-quoted; T-SQL does not support parameters in DDL.
//
// We check for an existing membership first because ALTER ROLE ADD MEMBER is
// idempotent — it succeeds silently even if the member is already in the role.
// Without this check, duplicate Terraform resources could silently claim the
// same underlying DB object, causing state corruption on destroy.
func (c *Connector) CreateRoleMember(ctx context.Context, rm *database_pkg.RoleMember) error {
	existing, err := c.GetRoleMember(ctx, rm.Role, rm.Member)
	if err != nil {
		return fmt.Errorf("checking existing membership: %w", err)
	}
	if existing != nil {
		return fmt.Errorf("member %q is already in role %q — import this resource instead of creating it", rm.Member, rm.Role)
	}

	stmt := fmt.Sprintf(
		"ALTER ROLE %s ADD MEMBER %s",
		quoteName(rm.Role), quoteName(rm.Member),
	)
	if err := withRetry(ctx, func() error {
		_, err := c.db.ExecContext(ctx, stmt)
		return err
	}); err != nil {
		return fmt.Errorf("ALTER ROLE ADD MEMBER: %w", err)
	}
	return nil
}

// DeleteRoleMember removes member from role using ALTER ROLE ... DROP MEMBER.
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

	stmt := fmt.Sprintf(
		"ALTER ROLE %s DROP MEMBER %s",
		quoteName(role), quoteName(member),
	)
	if err := withRetry(ctx, func() error {
		_, err := c.db.ExecContext(ctx, stmt)
		return err
	}); err != nil {
		return fmt.Errorf("ALTER ROLE DROP MEMBER: %w", err)
	}
	return nil
}
