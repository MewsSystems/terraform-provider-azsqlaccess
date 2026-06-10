// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package mssql

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	database "github.com/mews/terraform-provider-azsqlaccess/internal/database"
)

// fakeCred is a deterministic azcore.TokenCredential for unit tests so we
// never hit real Entra.
type fakeCred struct {
	token string
	err   error
	calls int
}

func (f *fakeCred) GetToken(_ context.Context, _ policy.TokenRequestOptions) (azcore.AccessToken, error) {
	f.calls++
	if f.err != nil {
		return azcore.AccessToken{}, f.err
	}
	return azcore.AccessToken{Token: f.token, ExpiresOn: time.Now().Add(time.Hour)}, nil
}

// mustConnector unwraps the DatabaseConnector returned by Factory.GetConnector
// or fails the test. Centralising the type assertion keeps individual tests
// linter-clean (forcetypeassert) without losing the panic-on-mismatch safety.
func mustConnector(t *testing.T, c database.DatabaseConnector) *Connector {
	t.Helper()
	cc, ok := c.(*Connector)
	if !ok {
		t.Fatalf("expected *Connector, got %T", c)
	}
	return cc
}

func TestBuildDSN_OmitsCredentialFields(t *testing.T) {
	// Auth is handled programmatically via the token provider — credentials
	// must NOT appear in the DSN under any circumstance.
	dsn := buildDSN("myserver.database.windows.net", "mydb")

	if !strings.HasPrefix(dsn, "sqlserver://myserver.database.windows.net") {
		t.Errorf("DSN should start with sqlserver scheme + host; got %s", dsn)
	}
	if !strings.Contains(dsn, "database=mydb") {
		t.Errorf("DSN should include target database; got %s", dsn)
	}
	for _, banned := range []string{"fedauth", "user+id", "user%20id", "tenant+id", "tenant%20id", "password"} {
		if strings.Contains(dsn, banned) {
			t.Errorf("DSN must not contain %q; got %s", banned, dsn)
		}
	}
}

func TestAcquireToken_HappyPath(t *testing.T) {
	cred := &fakeCred{token: "tok-abc"}
	got, err := acquireToken(cred)
	if err != nil {
		t.Fatalf("acquireToken: %v", err)
	}
	if got != "tok-abc" {
		t.Errorf("token = %q, want tok-abc", got)
	}
	if cred.calls != 1 {
		t.Errorf("GetToken calls = %d, want 1", cred.calls)
	}
}

func TestAcquireToken_WrapsError(t *testing.T) {
	cred := &fakeCred{err: errors.New("entra refused")}
	_, err := acquireToken(cred)
	if err == nil || !strings.Contains(err.Error(), "acquiring Azure token for mssql") {
		t.Fatalf("expected wrapped error, got %v", err)
	}
}

func TestFactory_GetConnector_CachesByKey(t *testing.T) {
	f := NewFactory(&fakeCred{token: "tok"})

	c1, err := f.GetConnector("server-a.example.com", "db1")
	if err != nil {
		t.Fatalf("GetConnector failed: %v", err)
	}
	c2, err := f.GetConnector("server-a.example.com", "db1")
	if err != nil {
		t.Fatalf("GetConnector failed: %v", err)
	}

	cc1 := mustConnector(t, c1)
	cc2 := mustConnector(t, c2)
	if cc1.db != cc2.db {
		t.Errorf("repeated GetConnector for same (server,database) must reuse the cached pool")
	}

	c3, err := f.GetConnector("server-a.example.com", "db2")
	if err != nil {
		t.Fatalf("GetConnector failed: %v", err)
	}
	cc3 := mustConnector(t, c3)
	if cc1.db == cc3.db {
		t.Errorf("different database keys must use distinct pools")
	}

	c4, err := f.GetConnector("server-b.example.com", "db1")
	if err != nil {
		t.Fatalf("GetConnector failed: %v", err)
	}
	cc4 := mustConnector(t, c4)
	if cc1.db == cc4.db {
		t.Errorf("different server keys must use distinct pools")
	}
}

func TestConnector_Close_IsNoOp(t *testing.T) {
	// Close must NOT close the cached pool — it is a no-op so resource callers
	// can defer conn.Close() without invalidating the Factory's cache.
	c := &Connector{}
	if err := c.Close(); err != nil {
		t.Fatalf("Close should return nil; got %v", err)
	}
}
