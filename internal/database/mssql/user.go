// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package mssql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/mews/terraform-provider-azsqlaccess/internal/database"
)

// quoteName wraps a SQL identifier in brackets and escapes embedded brackets.
// Use for all DDL identifiers (user names, schema names, role names).
// Never use for values — use sql.Named() parameters for those.
func quoteName(name string) string {
	out := make([]byte, 0, len(name)+2)
	for i := 0; i < len(name); i++ {
		if name[i] == ']' {
			out = append(out, ']', ']') // escape ] as ]]
		} else {
			out = append(out, name[i])
		}
	}
	return "[" + string(out) + "]"
}

func (c *Connector) CreateUser(ctx context.Context, user *database.User) error {
	// Azure SQL principal resolution strategy — driven by explicit type:
	//
	//   type = "user":
	//     → CREATE USER [upn] FROM EXTERNAL PROVIDER
	//     UPNs are globally unique in Entra. WITH OBJECT_ID is not used and would
	//     actively fail here (Azure SQL requires the name to begin with the display
	//     name, not the UPN).
	//
	//   type = "group" or "service_principal":
	//     → CREATE USER [display_name] FROM EXTERNAL PROVIDER WITH OBJECT_ID = '...'
	//     Display names are NOT unique in Entra (two groups or two MSIs can share a
	//     name). object_id is required for these types and guarantees unambiguous
	//     resolution.
	//
	// T-SQL does not support named parameters in DDL option clauses, so the UUID
	// must be inlined. We validate the UUID format first — a well-formed UUID
	// contains only hex digits and hyphens, making injection structurally impossible.

	var createSQL string
	switch user.Type {
	case "user":
		createSQL = fmt.Sprintf(
			"CREATE USER %s FROM EXTERNAL PROVIDER",
			quoteName(user.Name),
		)
	case "group", "service_principal":
		if _, err := uuid.Parse(user.ObjectID); err != nil {
			return fmt.Errorf("object_id %q is not a valid UUID: %w", user.ObjectID, err)
		}
		createSQL = fmt.Sprintf(
			"CREATE USER %s FROM EXTERNAL PROVIDER WITH OBJECT_ID = '%s'",
			quoteName(user.Name), user.ObjectID,
		)
	default:
		return fmt.Errorf("unknown principal type %q: must be user, group, or service_principal", user.Type)
	}

	// DDL operations (CREATE USER, ALTER USER) can be chosen as deadlock victims
	// by Azure SQL when multiple sessions run concurrent DDL. Retry on 1205.
	if err := withRetry(ctx, func() error {
		_, err := c.db.ExecContext(ctx, createSQL)
		return err
	}); err != nil {
		return translateCreateUserError(user.Name, err)
	}

	// Apply a non-default schema now that the user row exists.
	if user.DefaultSchema != "" && user.DefaultSchema != "dbo" {
		alterSQL := fmt.Sprintf(
			"ALTER USER %s WITH DEFAULT_SCHEMA = %s",
			quoteName(user.Name), quoteName(user.DefaultSchema),
		)
		if err := withRetry(ctx, func() error {
			_, err := c.db.ExecContext(ctx, alterSQL)
			return err
		}); err != nil {
			return fmt.Errorf("ALTER USER DEFAULT_SCHEMA: %w", err)
		}
	}

	// Read back computed fields (principal_id, object_id, resolved schema/language).
	return readUserInto(ctx, c.db, user)
}

func (c *Connector) GetUser(ctx context.Context, name string) (*database.User, error) {
	// Without this, "no rows" below could mean "not allowed to look" and the
	// caller would drop a live user from state.
	if err := c.CheckReadAccess(ctx, database.ReadScopeUser); err != nil {
		return nil, err
	}

	u := &database.User{Name: name}
	err := readUserInto(ctx, c.db, u)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil // resource gone outside Terraform
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (c *Connector) UpdateUser(ctx context.Context, user *database.User) error {
	schema := user.DefaultSchema
	if schema == "" {
		// Empty means "revert to engine default". MSSQL's default schema is "dbo".
		// Generating ALTER USER [x] WITH DEFAULT_SCHEMA = [] is invalid T-SQL and
		// would produce a syntax error — always fall back to "dbo" instead.
		schema = "dbo"
	}
	alterSQL := fmt.Sprintf(
		"ALTER USER %s WITH DEFAULT_SCHEMA = %s",
		quoteName(user.Name), quoteName(schema),
	)
	if err := withRetry(ctx, func() error {
		_, err := c.db.ExecContext(ctx, alterSQL)
		return err
	}); err != nil {
		return fmt.Errorf("ALTER USER DEFAULT_SCHEMA: %w", err)
	}
	return readUserInto(ctx, c.db, user)
}

func (c *Connector) DeleteUser(ctx context.Context, name string) error {
	return withRetry(ctx, func() error {
		_, err := c.db.ExecContext(ctx, fmt.Sprintf("DROP USER IF EXISTS %s", quoteName(name)))
		return err
	})
}

// translateCreateUserError converts well-known Azure SQL CREATE USER errors into
// actionable messages. Falls back to the raw error for anything else.
func translateCreateUserError(name string, err error) error {
	msg := err.Error()

	// "Principal name with object id '...' must begin with the original principal name"
	// This can still happen if the caller provides a display-name-format name that
	// does not actually match the principal's Entra display name.
	if strings.Contains(msg, "must begin with the original principal name") {
		return fmt.Errorf(
			"CREATE USER: Azure SQL rejected the name/object_id combination. "+
				"When object_id is set, the name must be the principal's Entra display name "+
				"(e.g. \"Juan Perez\"), not a suffix or alias. "+
				"If the principal is a user, try using the UPN (e.g. \"juan.perez@milanesa.com\") "+
				"without object_id instead: %w", err)
	}

	// "There is already an object named 'X' in the current database" or
	// "User or role 'X' already exists in the current database."
	if strings.Contains(msg, "already exists") {
		return fmt.Errorf(
			"CREATE USER: a user named %q already exists in the database. "+
				"If it was created outside Terraform, import it with: "+
				"terraform import <resource_address> <server>/<database>/%s: %w",
			name, name, err)
	}

	return fmt.Errorf("CREATE USER: %w", err)
}

// readUserInto queries sys.database_principals for user.Name and populates
// PrincipalID and DefaultSchema in-place.
// Returns sql.ErrNoRows if the user does not exist.
//
// ObjectID is intentionally NOT read back from the database: it always comes
// from config or the import ID, so state never depends on re-deriving it.
// (For contained Entra users the object ID is recoverable from the SID via
// CONVERT(uniqueidentifier, sid), but we deliberately don't rely on that.)
func readUserInto(ctx context.Context, db *sql.DB, user *database.User) error {
	const q = `
		SELECT
			principal_id,
			COALESCE(default_schema_name, '')
		FROM sys.database_principals
		WHERE name = @name
		  AND type IN ('E', 'X')`

	var schema string
	err := db.QueryRowContext(ctx, q, sql.Named("name", user.Name)).Scan(
		&user.PrincipalID,
		&schema,
	)
	if err != nil {
		return err
	}
	// Only overwrite if the database has a non-empty value.
	// Azure SQL returns NULL for default_schema_name on groups and MSIs,
	// which COALESCE maps to "".
	if schema != "" {
		user.DefaultSchema = schema
	}
	return nil
}
