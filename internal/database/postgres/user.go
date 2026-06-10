// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	database "github.com/mews/terraform-provider-azsqlaccess/internal/database"
)

// quoteIdentifier wraps a PostgreSQL identifier in double-quotes and escapes
// embedded double-quotes by doubling them. This is the PostgreSQL equivalent
// of MSSQL's quoteName() — use for all DDL identifiers (role names, schema names).
// Never use for values; those go through $1 parameterisation.
func quoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func (c *Connector) CreateUser(ctx context.Context, user *database.User) error {
	// Obtain the system database pool lazily — pgaadauth_create_principal only
	// exists in the "postgres" system database. The pool is cached by the Factory
	// so subsequent CreateUser calls on the same server reuse it; do not close it.
	sysPool, err := c.newSysPool()
	if err != nil {
		return fmt.Errorf("opening system database connection: %w", err)
	}

	// Pre-flight: verify that Microsoft Entra authentication is enabled on this
	// server (i.e. pgaadauth_create_principal exists in the postgres db).
	// Without this check the failure is SQLSTATE 42883 ("function does not
	// exist"), which is opaque.
	if err := checkPgaadauthExtension(ctx, sysPool); err != nil {
		return err
	}

	// pgaadauth_create_principal must be called against the "postgres" system
	// database — the function does not exist in user databases. PostgreSQL roles
	// are server-level, so the created role is immediately available in all
	// databases on the server.
	//
	// Two variants:
	//
	//   pgaadauth_create_principal(name, isAdmin, isMfa)
	//     Used for type=user. The UPN is globally unique in Entra so no OID is
	//     needed for disambiguation.
	//
	//   pgaadauth_create_principal_with_oid(name, objectId, objectType, isAdmin, isMfa)
	//     Used for type=group and type=service_principal. Display names are NOT
	//     unique in Entra (e.g. two groups called "db.reader"). The OID
	//     guarantees we bind the role to the correct principal. objectType maps:
	//       group              → 'group'
	//       service_principal  → 'service'
	//
	// Parameters: isAdmin=false (no superuser), isMfa=false (no MFA enforcement).
	switch user.Type {
	case "user":
		if _, err := sysPool.Exec(ctx,
			"SELECT pgaadauth_create_principal($1, false, false)",
			user.Name,
		); err != nil {
			return fmt.Errorf("pgaadauth_create_principal: %w", err)
		}
	case "group":
		if _, err := sysPool.Exec(ctx,
			"SELECT pgaadauth_create_principal_with_oid($1, $2, 'group', false, false)",
			user.Name, user.ObjectID,
		); err != nil {
			return fmt.Errorf("pgaadauth_create_principal_with_oid: %w", err)
		}
	case "service_principal":
		if _, err := sysPool.Exec(ctx,
			"SELECT pgaadauth_create_principal_with_oid($1, $2, 'service', false, false)",
			user.Name, user.ObjectID,
		); err != nil {
			return fmt.Errorf("pgaadauth_create_principal_with_oid: %w", err)
		}
	default:
		return fmt.Errorf("unknown principal type %q: must be user, group, or service_principal", user.Type)
	}

	// search_path is the PostgreSQL equivalent of MSSQL's DEFAULT_SCHEMA.
	// Only set it if the caller specified a non-default schema.
	if user.DefaultSchema != "" && user.DefaultSchema != "public" {
		stmt := fmt.Sprintf(
			"ALTER ROLE %s SET search_path TO %s",
			quoteIdentifier(user.Name), quoteIdentifier(user.DefaultSchema),
		)
		if _, err := c.pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("ALTER ROLE SET search_path: %w", err)
		}
	}

	// Normalise: if the caller did not specify a schema (DefaultSchema == ""),
	// store "public" — PostgreSQL's engine default — so state is always concrete.
	// This mirrors MSSQL's behaviour where readUserInto always returns "dbo".
	// Without this, a postgres user created without default_schema would show
	// default_schema = "" in state, which is misleading and diverges from reality.
	if user.DefaultSchema == "" {
		user.DefaultSchema = "public"
	}
	return readUserInto(ctx, c.pool, user)
}

func (c *Connector) GetUser(ctx context.Context, name string) (*database.User, error) {
	u := &database.User{Name: name}
	err := readUserInto(ctx, c.pool, u)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil // resource gone outside Terraform — caller will RemoveResource
	}
	if err != nil {
		return nil, err
	}
	// Normalise: readUserInto only populates PrincipalID (search_path is not
	// stored in pg_roles). Default to "public" so drift detection is stable
	// and import produces a valid initial state.
	if u.DefaultSchema == "" {
		u.DefaultSchema = "public"
	}
	return u, nil
}

func (c *Connector) UpdateUser(ctx context.Context, user *database.User) error {
	var stmt string
	if user.DefaultSchema == "" {
		// Empty means "revert to engine default". Using SET search_path TO ''
		// would set a literally empty search path, breaking all unqualified
		// queries. RESET is the correct SQL to restore the role's default.
		stmt = fmt.Sprintf("ALTER ROLE %s RESET search_path", quoteIdentifier(user.Name))
	} else {
		stmt = fmt.Sprintf(
			"ALTER ROLE %s SET search_path TO %s",
			quoteIdentifier(user.Name), quoteIdentifier(user.DefaultSchema),
		)
	}
	if _, err := c.pool.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("ALTER ROLE search_path: %w", err)
	}
	return readUserInto(ctx, c.pool, user)
}

func (c *Connector) DeleteUser(ctx context.Context, name string) error {
	return withRetry(ctx, func() error {
		_, err := c.pool.Exec(ctx, fmt.Sprintf("DROP ROLE IF EXISTS %s", quoteIdentifier(name)))
		return err
	})
}

// checkPgaadauthExtension verifies that Microsoft Entra authentication is
// enabled on this Azure PostgreSQL Flexible Server. It does so by checking
// whether pgaadauth_create_principal exists in pg_proc.
//
// pgaadauth is NOT a user-installable extension — it is a built-in Azure
// capability that becomes available when Entra auth is enabled at the server
// level. It never appears in pg_extension and cannot be installed via
// CREATE EXTENSION. The raw failure when it is absent is SQLSTATE 42883
// ("function does not exist"), which looks like a provider bug rather than a
// setup problem — hence this targeted pre-flight check.
//
// To enable Entra auth on the server:
//
//	Azure Portal → PostgreSQL Flexible Server → Authentication
//	→ Authentication method: "PostgreSQL and Microsoft Entra authentication"
//	→ Set a Microsoft Entra admin → Save
func checkPgaadauthExtension(ctx context.Context, sysPool pgxConn) error {
	var count int
	err := sysPool.QueryRow(ctx,
		"SELECT COUNT(*) FROM pg_proc WHERE proname = 'pgaadauth_create_principal'",
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("checking for pgaadauth_create_principal: %w", err)
	}
	if count == 0 {
		// Multi-line user-facing diagnostic; staticcheck's ST1005 (lowercase
		// first letter, no trailing punctuation) does not fit a help text whose
		// first word is the proper noun "Microsoft".
		return errors.New( //nolint:staticcheck
			"Microsoft Entra authentication is not enabled on this Azure PostgreSQL Flexible Server\n\n" +
				"Enable it in the Azure Portal:\n" +
				"  1. Go to your PostgreSQL Flexible Server → Authentication\n" +
				"  2. Set Authentication method to \"PostgreSQL and Microsoft Entra authentication\"\n" +
				"  3. Set a Microsoft Entra admin\n" +
				"  4. Save\n\n" +
				"Once enabled, pgaadauth_create_principal becomes available automatically " +
				"in all databases — no CREATE EXTENSION step is required",
		)
	}
	return nil
}

// readUserInto queries pg_roles for user.Name and populates PrincipalID.
// Returns pgx.ErrNoRows if the role does not exist.
//
// DefaultLanguage is not populated — PostgreSQL has no equivalent concept;
// the resource layer always writes "" to state.
//
// DefaultSchema is intentionally not read back here: the search_path role
// setting lives in pg_db_role_setting and is awkward to retrieve reliably.
// Instead, CreateUser and GetUser normalise an unset schema to "public" (the
// engine default), so state stays concrete without querying it.
func readUserInto(ctx context.Context, pool pgxConn, user *database.User) error {
	return pool.QueryRow(ctx,
		"SELECT oid::bigint FROM pg_roles WHERE rolname = $1",
		user.Name,
	).Scan(&user.PrincipalID)
}
