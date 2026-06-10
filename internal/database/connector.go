// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package database

import "context"

// DatabaseConnector is the per-server-and-database abstraction that resources talk to.
// It is scoped to a single (server, database) pair at construction time, so methods
// do not take a database parameter — the connector already knows where it is pointed.
//
// Implemented by pkg/database/mssql (and future pkg/database/postgres).
// Resources never import engine-specific packages.
type DatabaseConnector interface {
	GetUser(ctx context.Context, name string) (*User, error)
	CreateUser(ctx context.Context, user *User) error
	UpdateUser(ctx context.Context, user *User) error
	DeleteUser(ctx context.Context, name string) error

	GetRoleMember(ctx context.Context, role, member string) (*RoleMember, error)
	CreateRoleMember(ctx context.Context, rm *RoleMember) error
	DeleteRoleMember(ctx context.Context, role, member string) error

	// Close releases the underlying connection pool.
	Close() error
}

// ConnectorFactory is what the provider stores in ResourceData.
// It holds the Entra credentials once and creates per-(server, database) connectors
// on demand. The database is included here so the DSN targets it directly —
// this avoids requiring access to `master`, which regular Entra users cannot connect to.
type ConnectorFactory interface {
	GetConnector(server, database string) (DatabaseConnector, error)
}
