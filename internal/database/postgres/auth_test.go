// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package postgres

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/jackc/pgx/v5"
)

// fakeCred is a deterministic azcore.TokenCredential for unit tests. It avoids
// pulling in azidentity or any real Entra round trip.
type fakeCred struct {
	token string
	err   error
}

func (f *fakeCred) GetToken(_ context.Context, _ policy.TokenRequestOptions) (azcore.AccessToken, error) {
	if f.err != nil {
		return azcore.AccessToken{}, f.err
	}
	return azcore.AccessToken{Token: f.token, ExpiresOn: time.Now().Add(time.Hour)}, nil
}

func TestApplyTokenAuth_HappyPath(t *testing.T) {
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"upn":"juan.perez@milanesa.com"}`))
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"typ":"JWT"}`))
	sig := base64.RawURLEncoding.EncodeToString([]byte("sig"))
	jwt := header + "." + payload + "." + sig

	cred := &fakeCred{token: jwt}
	cc := &pgx.ConnConfig{}
	if err := applyTokenAuth(context.Background(), cred, cc, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cc.User != "juan.perez@milanesa.com" {
		t.Errorf("User = %q, want juan.perez@milanesa.com", cc.User)
	}
	if cc.Password != jwt {
		t.Errorf("Password should equal the JWT")
	}
}

func TestApplyTokenAuth_GetTokenError(t *testing.T) {
	cred := &fakeCred{err: errors.New("forbidden")}
	cc := &pgx.ConnConfig{}
	err := applyTokenAuth(context.Background(), cred, cc, "")
	if err == nil {
		t.Fatalf("expected error from GetToken to surface")
	}
	if !strings.Contains(err.Error(), "acquiring Azure token") {
		t.Errorf("error should be wrapped; got %v", err)
	}
}

func TestApplyTokenAuth_InvalidJWT(t *testing.T) {
	cred := &fakeCred{token: "not-a-valid-jwt"}
	cc := &pgx.ConnConfig{}
	err := applyTokenAuth(context.Background(), cred, cc, "")
	if err == nil {
		t.Fatalf("expected error from JWT parsing to surface")
	}
	if !strings.Contains(err.Error(), "resolving identity from Azure token") {
		t.Errorf("error should be wrapped; got %v", err)
	}
}

func TestApplyTokenAuth_LoginUsernameOverridesTokenIdentity(t *testing.T) {
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"appid":"00000000-0000-0000-0000-000000000000"}`))
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"typ":"JWT"}`))
	sig := base64.RawURLEncoding.EncodeToString([]byte("sig"))
	jwt := header + "." + payload + "." + sig

	cred := &fakeCred{token: jwt}
	cc := &pgx.ConnConfig{}
	if err := applyTokenAuth(context.Background(), cred, cc, "db.reader"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cc.User != "db.reader" {
		t.Errorf("User = %q, want db.reader", cc.User)
	}
	// The token still belongs to the caller — only the assumed role changes.
	if cc.Password != jwt {
		t.Errorf("Password should equal the JWT")
	}
}

// A login_username must be usable even when the token carries no identity claim
// the provider knows how to read, since that claim is no longer consulted.
func TestApplyTokenAuth_LoginUsernameSkipsClaimParsing(t *testing.T) {
	cred := &fakeCred{token: "not-a-valid-jwt"}
	cc := &pgx.ConnConfig{}
	if err := applyTokenAuth(context.Background(), cred, cc, "db.reader"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cc.User != "db.reader" {
		t.Errorf("User = %q, want db.reader", cc.User)
	}
}
