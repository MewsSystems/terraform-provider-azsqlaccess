// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package mssql

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	database_pkg "github.com/mews/terraform-provider-azsqlaccess/internal/database"
	mssqldriver "github.com/microsoft/go-mssqldb"
)

var _ database_pkg.ConnectorFactory = (*Factory)(nil)
var _ database_pkg.DatabaseConnector = (*Connector)(nil)

// tokenScope is the OAuth2 audience for Azure SQL. All Entra tokens used to
// authenticate against the server must target this resource.
const tokenScope = "https://database.windows.net/.default"

// tokenAcquireTimeout bounds a single token-provider invocation. The go-mssqldb
// provider callback has no context, so this guards against an unresponsive Entra
// endpoint stalling a new connection indefinitely.
const tokenAcquireTimeout = 30 * time.Second

// Factory holds the shared Entra credential and a cache of *sql.DB pools keyed
// by "server\x00database". Pools are reused across all CRUD operations on the
// same (server, database) pair within a single Terraform apply — avoiding
// repeated TCP connect + Entra token acquisition on every resource operation.
// mu protects pools against concurrent access during parallel applies.
type Factory struct {
	cred  azcore.TokenCredential
	mu    sync.Mutex
	pools map[string]*sql.DB
}

// NewFactory wraps a pre-built credential. The credential is constructed once
// by the provider via database.BuildEntraCredential and shared with the
// postgres factory so the auth path is identical across engines.
func NewFactory(cred azcore.TokenCredential) *Factory {
	return &Factory{cred: cred, pools: make(map[string]*sql.DB)}
}

// GetConnector returns a Connector backed by a cached *sql.DB for the given
// (server, database) pair, creating the pool on first call.
// Including the database in the DSN means the initial TCP connection targets
// it directly — this avoids requiring access to `master`, which regular
// Entra users (non-admins) cannot connect to on Azure SQL.
//
// Authentication uses go-mssqldb's access-token connector: every new
// underlying connection invokes tokenProvider, which calls our shared
// azcore.TokenCredential. This keeps the same credential surface across MSSQL
// and Postgres and lets ARM_USE_OIDC / OIDC token-file flows work uniformly.
func (f *Factory) GetConnector(server, database string) (database_pkg.DatabaseConnector, error) {
	key := server + "\x00" + database

	f.mu.Lock()
	defer f.mu.Unlock()

	if db, ok := f.pools[key]; ok {
		return &Connector{db: db}, nil
	}

	dsn := buildDSN(server, database)
	cred := f.cred
	connector, err := mssqldriver.NewAccessTokenConnector(dsn, func() (string, error) {
		return acquireToken(cred)
	})
	if err != nil {
		return nil, fmt.Errorf("building connector for %s/%s: %w", server, database, err)
	}

	db := sql.OpenDB(connector)
	f.pools[key] = db
	return &Connector{db: db}, nil
}

// acquireToken fetches a fresh Azure SQL access token via the shared
// credential. Each call is independently bounded by tokenAcquireTimeout so a
// stuck Entra endpoint cannot deadlock a new connection setup.
func acquireToken(cred azcore.TokenCredential) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), tokenAcquireTimeout)
	defer cancel()
	tok, err := cred.GetToken(ctx, policy.TokenRequestOptions{Scopes: []string{tokenScope}})
	if err != nil {
		return "", fmt.Errorf("acquiring Azure token for mssql: %w", err)
	}
	return tok.Token, nil
}

// buildDSN assembles the sqlserver:// DSN. fedauth and credential fields are
// intentionally absent — auth is handled programmatically by the token
// provider passed to mssqldriver.NewAccessTokenConnector.
func buildDSN(server, database string) string {
	u := &url.URL{Scheme: "sqlserver", Host: server}
	q := u.Query()
	q.Set("database", database)
	u.RawQuery = q.Encode()
	return u.String()
}

// Connector holds a *sql.DB scoped to one (server, database) pair.
// The pool is owned by the Factory and shared across CRUD calls — do not
// close it here. Close is a no-op so that resource callers can defer
// conn.Close() without invalidating the cached pool.
type Connector struct {
	db *sql.DB
}

func (c *Connector) Close() error {
	return nil // pool lifecycle is owned by Factory
}
