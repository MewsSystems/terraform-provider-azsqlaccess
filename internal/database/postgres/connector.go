// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	database_pkg "github.com/mews/terraform-provider-azsqlaccess/internal/database"
)

// pgxConn is the minimal pgx pool surface used by Connector. *pgxpool.Pool
// satisfies it in production; pgxmock.PgxPoolIface satisfies it in tests.
// Keeping the interface tiny avoids accidentally widening the contract.
type pgxConn interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

var _ database_pkg.ConnectorFactory = (*Factory)(nil)
var _ database_pkg.DatabaseConnector = (*Connector)(nil)

// tokenScope is the OAuth2 audience for Azure Database for PostgreSQL.
// All Entra tokens used as passwords must target this resource.
const tokenScope = "https://ossrdbms-aad.database.windows.net/.default"

// Factory holds the resolved Entra credential and a cache of pgxpool.Pool
// instances keyed by "server\x00database". Pools are reused across all CRUD
// operations on the same (server, database) pair — including the "postgres"
// system database pool used by CreateUser — avoiding repeated Entra token
// acquisition and TCP handshakes per resource operation.
// mu protects pools against concurrent access during parallel applies.
// loginUsername, when non-empty, overrides the token-derived connection role
// name — see applyTokenAuth.
type Factory struct {
	cred          azcore.TokenCredential
	loginUsername string
	mu            sync.Mutex
	pools         map[string]*pgxpool.Pool
}

// NewFactory wraps a pre-built credential. The credential is constructed once
// by the provider via database.BuildEntraCredential, then shared across both
// engine factories so all auth flows through a single source of truth.
//
// loginUsername is the provider's optional login_username: pass "" to keep the
// default behaviour of connecting as the token's own principal.
func NewFactory(cred azcore.TokenCredential, loginUsername string) *Factory {
	return &Factory{
		cred:          cred,
		loginUsername: loginUsername,
		pools:         make(map[string]*pgxpool.Pool),
	}
}

// GetConnector returns a Connector backed by a cached pgxpool for the given
// (server, database) pair, creating the pool on first call.
// Before each new connection the pool acquires a fresh Entra token and:
//   - injects it as the connection password (required by pgaadauth)
//   - sets the connection username to the provider's login_username, or, when
//     that is unset, to the caller identity extracted from the JWT claims
//
// The system database pool ("postgres") is NOT opened here. It is obtained
// lazily inside CreateUser via the same cache — the only operation that needs
// it. Role member operations never touch the system database.
func (f *Factory) GetConnector(server, database string) (database_pkg.DatabaseConnector, error) {
	pool, err := f.cachedPool(server, database)
	if err != nil {
		return nil, err
	}

	// The newSysPool closure also goes through the cache, so repeated CreateUser
	// calls on the same server reuse the same system pool.
	f2 := f
	return &Connector{
		pool: pool,
		newSysPool: func() (pgxConn, error) {
			return f2.cachedPool(server, "postgres")
		},
	}, nil
}

// cachedPool returns an existing pool for (server, database) or creates and
// caches a new one. Safe for concurrent use during parallel Terraform applies.
func (f *Factory) cachedPool(server, database string) (*pgxpool.Pool, error) {
	key := server + "\x00" + database

	f.mu.Lock()
	defer f.mu.Unlock()

	if pool, ok := f.pools[key]; ok {
		return pool, nil
	}

	pool, err := f.newPool(server, database)
	if err != nil {
		return nil, err
	}
	f.pools[key] = pool
	return pool, nil
}

// newPool creates a pgxpool for the given server and database.
// It injects a fresh Entra token (and the derived username) before each
// new connection, so tokens are always valid across long-running applies.
func (f *Factory) newPool(server, database string) (*pgxpool.Pool, error) {
	// No user= in the DSN — it is set in BeforeConnect, from login_username when
	// configured and from the JWT claims otherwise.
	connStr := fmt.Sprintf("host=%s dbname=%s sslmode=require", server, database)
	config, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("parsing postgres config for %s/%s: %w", server, database, err)
	}

	cred := f.cred                   // capture for closure
	loginUsername := f.loginUsername // capture for closure
	config.BeforeConnect = func(ctx context.Context, connConfig *pgx.ConnConfig) error {
		return applyTokenAuth(ctx, cred, connConfig, loginUsername)
	}

	// pgxpool is lazy — no connections are opened until first use.
	return pgxpool.NewWithConfig(context.Background(), config)
}

// applyTokenAuth acquires an Entra token for the PostgreSQL audience and writes
// the derived username and the token (as the password) onto connConfig.
// Extracted from newPool's BeforeConnect closure so it can be unit-tested in
// isolation against a fake azcore.TokenCredential.
//
// Username derivation, when loginUsername is empty:
//   - upn / preferred_username for user accounts
//   - appid for service principals and managed identities
//
// A non-empty loginUsername is used verbatim instead. This is what makes group
// login work: pgaadauth has no server-side group expansion, so a caller whose
// only admin grant comes from Entra group membership must connect as the group's
// role name while still presenting its own token. The token is never derived
// from loginUsername — only the role being assumed is.
//
// Token acquisition uses the fixed PostgreSQL audience scope. The token IS the
// password — pgaadauth on the server validates it on each new connection.
func applyTokenAuth(ctx context.Context, cred azcore.TokenCredential, connConfig *pgx.ConnConfig, loginUsername string) error {
	token, err := cred.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: []string{tokenScope},
	})
	if err != nil {
		return fmt.Errorf("acquiring Azure token for postgres: %w", err)
	}

	username := loginUsername
	if username == "" {
		username, err = usernameFromToken(token.Token)
		if err != nil {
			return fmt.Errorf("resolving identity from Azure token: %w", err)
		}
	}

	connConfig.User = username
	connConfig.Password = token.Token
	return nil
}

// usernameFromToken extracts the Entra principal identity from a JWT access token.
//
// Azure tokens issued to different principal types carry the identity in different claims:
//   - "upn"                — work/school user accounts (e.g. user@tenant.com)
//   - "preferred_username" — fallback for user accounts (same value, different issuers)
//   - "appid"              — service principals and managed identities (client ID UUID)
//
// The function tries each claim in order and returns the first non-empty value.
// No cryptographic verification is performed — the token was just issued by Azure
// so we trust its payload for the purpose of reading our own identity.
func usernameFromToken(tokenStr string) (string, error) {
	// A JWT is three base64url segments separated by dots: header.payload.signature
	parts := strings.SplitN(tokenStr, ".", 3)
	if len(parts) != 3 {
		return "", fmt.Errorf("unexpected JWT format: expected 3 segments, got %d", len(parts))
	}

	// base64.RawURLEncoding handles the URL-safe alphabet and no-padding variant
	// used by JWT. Standard base64 would fail on tokens containing - or _.
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("base64-decoding JWT payload: %w", err)
	}

	var claims struct {
		UPN               string `json:"upn"`
		PreferredUsername string `json:"preferred_username"`
		AppID             string `json:"appid"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", fmt.Errorf("parsing JWT claims: %w", err)
	}

	if claims.UPN != "" {
		return claims.UPN, nil
	}
	if claims.PreferredUsername != "" {
		return claims.PreferredUsername, nil
	}
	if claims.AppID != "" {
		return claims.AppID, nil
	}
	return "", fmt.Errorf("JWT contains no usable identity claim (upn, preferred_username, or appid)")
}

// Connector holds one pool for the target database plus a factory closure for
// the system database pool. Both pools are cached by the Factory — do not close
// them here. Close is a no-op so that resource callers can defer conn.Close()
// without invalidating the cached pool.
type Connector struct {
	pool       pgxConn
	newSysPool func() (pgxConn, error)
}

func (c *Connector) Close() error {
	return nil // pool lifecycle is owned by Factory
}
