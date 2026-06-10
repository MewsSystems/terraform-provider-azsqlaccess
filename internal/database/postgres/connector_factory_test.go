// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package postgres

import (
	"strings"
	"testing"

	database_pkg "github.com/mews/terraform-provider-azsqlaccess/internal/database"
)

// newTestFactory builds a Factory backed by a deterministic fakeCred so
// factory-level tests do not depend on the host's Azure environment. Pool
// creation in pgx is lazy — no network calls happen during these tests.
func newTestFactory(_ *testing.T) *Factory {
	return NewFactory(&fakeCred{token: "test-token"})
}

func TestNewFactory_StoresCredentialAndInitsPools(t *testing.T) {
	cred := &fakeCred{token: "test"}
	f := NewFactory(cred)
	if f == nil || f.cred == nil {
		t.Fatalf("Factory or its credential is nil")
	}
	if f.cred != cred {
		t.Errorf("Factory should hold the credential it was constructed with")
	}
	if f.pools == nil {
		t.Fatalf("pool cache should be initialised")
	}
}

func TestConnector_Close_IsNoOp(t *testing.T) {
	c := &Connector{}
	if err := c.Close(); err != nil {
		t.Fatalf("Close should be a no-op; got %v", err)
	}
}

// mustConnector unwraps the DatabaseConnector returned by Factory.GetConnector
// or fails the test. Centralising the type assertion keeps individual tests
// linter-clean (forcetypeassert) without losing the panic-on-mismatch safety.
func mustConnector(t *testing.T, c database_pkg.DatabaseConnector) *Connector {
	t.Helper()
	cc, ok := c.(*Connector)
	if !ok {
		t.Fatalf("expected *Connector, got %T", c)
	}
	return cc
}

func TestFactory_GetConnector_ReturnsConnector(t *testing.T) {
	f := newTestFactory(t)
	c, err := f.GetConnector("server.example.com", "mydb")
	if err != nil {
		t.Fatalf("GetConnector: %v", err)
	}
	if c == nil {
		t.Fatalf("expected non-nil connector")
	}
	cc := mustConnector(t, c)
	if cc.pool == nil {
		t.Fatalf("connector pool should be set")
	}
	if cc.newSysPool == nil {
		t.Fatalf("connector newSysPool should be set")
	}
	t.Cleanup(func() {
		for _, p := range f.pools {
			p.Close()
		}
	})
}

func TestFactory_GetConnector_CachesByKey(t *testing.T) {
	f := newTestFactory(t)
	t.Cleanup(func() {
		for _, p := range f.pools {
			p.Close()
		}
	})

	c1, _ := f.GetConnector("server-a.example.com", "db1")
	c2, _ := f.GetConnector("server-a.example.com", "db1")
	cc1, cc2 := mustConnector(t, c1), mustConnector(t, c2)
	if cc1.pool != cc2.pool {
		t.Errorf("repeated GetConnector for same key must reuse pool")
	}

	c3, _ := f.GetConnector("server-a.example.com", "db2")
	cc3 := mustConnector(t, c3)
	if cc1.pool == cc3.pool {
		t.Errorf("different database keys must use distinct pools")
	}

	c4, _ := f.GetConnector("server-b.example.com", "db1")
	cc4 := mustConnector(t, c4)
	if cc1.pool == cc4.pool {
		t.Errorf("different server keys must use distinct pools")
	}

	if len(f.pools) != 3 {
		t.Errorf("pool cache size = %d, want 3", len(f.pools))
	}
}

func TestFactory_GetConnector_ParseConfigError(t *testing.T) {
	// An unterminated single quote in dbname breaks pgxpool.ParseConfig.
	// This drives cachedPool → newPool's error branch and the wrapped error
	// that GetConnector surfaces.
	f := newTestFactory(t)
	t.Cleanup(func() {
		for _, p := range f.pools {
			p.Close()
		}
	})

	_, err := f.GetConnector("server.example.com", "'unterminated")
	if err == nil {
		t.Fatalf("expected ParseConfig error to surface")
	}
	if !strings.Contains(err.Error(), "parsing postgres config") {
		t.Errorf("error should be wrapped; got %v", err)
	}
}

func TestFactory_NewSysPool_ReusesPostgresPool(t *testing.T) {
	f := newTestFactory(t)
	t.Cleanup(func() {
		for _, p := range f.pools {
			p.Close()
		}
	})

	c, _ := f.GetConnector("server.example.com", "mydb")
	cc := mustConnector(t, c)

	sys1, err := cc.newSysPool()
	if err != nil {
		t.Fatalf("newSysPool: %v", err)
	}
	sys2, err := cc.newSysPool()
	if err != nil {
		t.Fatalf("newSysPool: %v", err)
	}
	if sys1 != sys2 {
		t.Errorf("newSysPool should return the cached pool")
	}
	if sys1 == cc.pool {
		t.Errorf("system pool must differ from target-db pool")
	}
}
