// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package database

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOIDCEnabled(t *testing.T) {
	cases := map[string]bool{
		"":      false,
		"0":     false,
		"false": false,
		"no":    false,
		"1":     true,
		"true":  true,
		"TRUE":  true,
		" yes ": true,
		"on":    true,
	}
	for v, want := range cases {
		t.Run(v, func(t *testing.T) {
			t.Setenv("ARM_USE_OIDC", v)
			if got := oidcEnabled(); got != want {
				t.Errorf("oidcEnabled() with ARM_USE_OIDC=%q = %v, want %v", v, got, want)
			}
		})
	}
}

func TestBuildEntraCredential_ServicePrincipal(t *testing.T) {
	clearOIDCEnv(t)
	cred, err := BuildEntraCredential(
		"00000000-0000-0000-0000-000000000000",
		"11111111-1111-1111-1111-111111111111",
		"secret",
	)
	if err != nil {
		t.Fatalf("BuildEntraCredential: %v", err)
	}
	if cred == nil {
		t.Fatal("credential is nil")
	}
}

func TestBuildEntraCredential_OIDC_RequiresTenantAndClient(t *testing.T) {
	clearOIDCEnv(t)
	t.Setenv("ARM_USE_OIDC", "true")
	t.Setenv("ARM_TENANT_ID", "")
	t.Setenv("ARM_CLIENT_ID", "")
	t.Setenv("AZURE_TENANT_ID", "")
	t.Setenv("AZURE_CLIENT_ID", "")

	if _, err := BuildEntraCredential("", "", ""); err == nil {
		t.Fatal("expected error when ARM_USE_OIDC=true without tenant/client IDs")
	}
}

func TestBuildEntraCredential_OIDC_NoSource(t *testing.T) {
	clearOIDCEnv(t)
	t.Setenv("ARM_USE_OIDC", "true")
	t.Setenv("ARM_TENANT_ID", "00000000-0000-0000-0000-000000000000")
	t.Setenv("ARM_CLIENT_ID", "11111111-1111-1111-1111-111111111111")

	if _, err := BuildEntraCredential("", "", ""); err == nil ||
		!strings.Contains(err.Error(), "no OIDC token source") {
		t.Fatalf("expected 'no OIDC token source' error, got %v", err)
	}
}

func TestBuildEntraCredential_OIDC_ExplicitToken(t *testing.T) {
	clearOIDCEnv(t)
	t.Setenv("ARM_USE_OIDC", "true")
	t.Setenv("ARM_TENANT_ID", "00000000-0000-0000-0000-000000000000")
	t.Setenv("ARM_CLIENT_ID", "11111111-1111-1111-1111-111111111111")
	t.Setenv("ARM_OIDC_TOKEN", "dummy-jwt-value")

	cred, err := BuildEntraCredential("", "", "")
	if err != nil {
		t.Fatalf("BuildEntraCredential: %v", err)
	}
	if cred == nil {
		t.Fatal("credential is nil")
	}
}

func TestBuildEntraCredential_OIDC_TokenFile(t *testing.T) {
	clearOIDCEnv(t)
	path := filepath.Join(t.TempDir(), "oidc.token")
	if err := os.WriteFile(path, []byte("file-jwt-value\n"), 0o600); err != nil {
		t.Fatalf("writing token file: %v", err)
	}
	t.Setenv("ARM_USE_OIDC", "true")
	t.Setenv("ARM_TENANT_ID", "00000000-0000-0000-0000-000000000000")
	t.Setenv("ARM_CLIENT_ID", "11111111-1111-1111-1111-111111111111")
	t.Setenv("ARM_OIDC_TOKEN_FILE_PATH", path)

	cred, err := BuildEntraCredential("", "", "")
	if err != nil {
		t.Fatalf("BuildEntraCredential: %v", err)
	}
	if cred == nil {
		t.Fatal("credential is nil")
	}
}

func TestReadTokenFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tok")
	if err := os.WriteFile(path, []byte("  jwt-value\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := readTokenFile(path)
	if err != nil {
		t.Fatalf("readTokenFile: %v", err)
	}
	if got != "jwt-value" {
		t.Errorf("token = %q, want %q (whitespace trimmed)", got, "jwt-value")
	}

	empty := filepath.Join(t.TempDir(), "empty")
	if err := os.WriteFile(empty, []byte("   \n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := readTokenFile(empty); err == nil {
		t.Error("expected error for empty token file")
	}

	if _, err := readTokenFile(filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
		t.Error("expected error for missing token file")
	}
}

func TestFetchGitHubOIDCToken_Success(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("audience"); got != oidcExchangeAudience {
			t.Errorf("audience query = %q, want %q", got, oidcExchangeAudience)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-request-token" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer test-request-token")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":"jwt-id-token"}`))
	}))
	defer srv.Close()

	token, err := fetchGitHubOIDCToken(context.Background(), srv.Client(), srv.URL, "test-request-token")
	if err != nil {
		t.Fatalf("fetchGitHubOIDCToken: %v", err)
	}
	if token != "jwt-id-token" {
		t.Errorf("token = %q, want %q", token, "jwt-id-token")
	}
}

func TestFetchGitHubOIDCToken_RejectsNonHTTPS(t *testing.T) {
	if _, err := fetchGitHubOIDCToken(context.Background(), http.DefaultClient,
		"http://example.com/token", "tok"); err == nil ||
		!strings.Contains(err.Error(), "must be https") {
		t.Fatalf("expected https-scheme error, got %v", err)
	}
}

func TestFetchGitHubOIDCToken_HTTPError(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	_, err := fetchGitHubOIDCToken(context.Background(), srv.Client(), srv.URL, "tok")
	if err == nil || !strings.Contains(err.Error(), "HTTP 403") {
		t.Fatalf("expected HTTP 403 error, got %v", err)
	}
}

func TestFetchGitHubOIDCToken_EmptyValue(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"value":""}`))
	}))
	defer srv.Close()

	_, err := fetchGitHubOIDCToken(context.Background(), srv.Client(), srv.URL, "tok")
	if err == nil || !strings.Contains(err.Error(), "missing token value") {
		t.Fatalf("expected missing-value error, got %v", err)
	}
}

func TestFetchGitHubOIDCToken_MalformedJSON(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer srv.Close()

	_, err := fetchGitHubOIDCToken(context.Background(), srv.Client(), srv.URL, "tok")
	if err == nil || !strings.Contains(err.Error(), "decoding OIDC response") {
		t.Fatalf("expected decode error, got %v", err)
	}
}

// clearOIDCEnv unsets every env var BuildEntraCredential consults so each test
// starts from a known state. t.Setenv restores prior values at cleanup.
func clearOIDCEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"ARM_USE_OIDC",
		"ARM_TENANT_ID",
		"ARM_CLIENT_ID",
		"ARM_OIDC_TOKEN",
		"ARM_OIDC_TOKEN_FILE_PATH",
		"ACTIONS_ID_TOKEN_REQUEST_URL",
		"ACTIONS_ID_TOKEN_REQUEST_TOKEN",
		"AZURE_TENANT_ID",
		"AZURE_CLIENT_ID",
		"AZURE_CLIENT_SECRET",
		"AZURE_FEDERATED_TOKEN_FILE",
	} {
		t.Setenv(k, "")
	}
}
