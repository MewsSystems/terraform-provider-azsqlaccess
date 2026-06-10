// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package database

// User represents a contained Entra identity inside a specific database.
// The database is not stored here — it lives in the connector that was used
// to create/read/update/delete the user.
//
// Type drives the SQL path on both engines:
//   - "user"              → UPN is unique; no object_id needed.
//   - "group"             → display name; object_id required for disambiguation.
//   - "service_principal" → display name; object_id required for disambiguation.
//
// PrincipalID is always computed from the DB.
type User struct {
	Name          string
	Type          string // "user" | "group" | "service_principal"
	DefaultSchema string
	ObjectID      string // empty for type=user; required for group/service_principal
	PrincipalID   int64  // computed
}

// RoleMember represents a single role → member grant inside a database.
// The database is scoped in the connector, not stored here.
// There is no Update operation — the resource is either present or absent.
type RoleMember struct {
	Role   string
	Member string
}
