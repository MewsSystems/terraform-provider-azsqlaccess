// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package postgres

import (
	"encoding/base64"
	"strings"
	"testing"
)

// makeJWT assembles a JWT-shaped string with the given payload JSON and dummy
// header/signature segments. Used to drive usernameFromToken without minting
// real Azure tokens.
func makeJWT(payloadJSON string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"typ":"JWT","alg":"none"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(payloadJSON))
	sig := base64.RawURLEncoding.EncodeToString([]byte("not-real-signature"))
	return header + "." + payload + "." + sig
}

func TestUsernameFromToken_UPN(t *testing.T) {
	got, err := usernameFromToken(makeJWT(`{"upn":"juan.perez@milanesa.com"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "juan.perez@milanesa.com" {
		t.Fatalf("got %q want juan.perez@milanesa.com", got)
	}
}

func TestUsernameFromToken_PreferredUsername(t *testing.T) {
	got, err := usernameFromToken(makeJWT(`{"preferred_username":"juan.perez@milanesa.com"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "juan.perez@milanesa.com" {
		t.Fatalf("got %q want juan.perez@milanesa.com", got)
	}
}

func TestUsernameFromToken_AppID(t *testing.T) {
	got, err := usernameFromToken(makeJWT(`{"appid":"00000000-0000-0000-0000-000000000000"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "00000000-0000-0000-0000-000000000000" {
		t.Fatalf("got %q want appid uuid", got)
	}
}

func TestUsernameFromToken_PriorityUPNOverPreferred(t *testing.T) {
	// upn wins over preferred_username when both are present.
	got, err := usernameFromToken(makeJWT(`{"upn":"a@example.com","preferred_username":"b@example.com"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "a@example.com" {
		t.Fatalf("expected upn to win, got %q", got)
	}
}

func TestUsernameFromToken_PriorityPreferredOverAppID(t *testing.T) {
	got, err := usernameFromToken(makeJWT(`{"preferred_username":"x@example.com","appid":"00000000-0000-0000-0000-000000000000"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "x@example.com" {
		t.Fatalf("expected preferred_username to win, got %q", got)
	}
}

func TestUsernameFromToken_NoUsableClaim(t *testing.T) {
	_, err := usernameFromToken(makeJWT(`{"sub":"abc","aud":"def"}`))
	if err == nil {
		t.Fatalf("expected error when no identity claim present")
	}
	if !strings.Contains(err.Error(), "no usable identity claim") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestUsernameFromToken_BadFormat_TooFewSegments(t *testing.T) {
	_, err := usernameFromToken("only.two")
	if err == nil {
		t.Fatalf("expected error for malformed JWT")
	}
	if !strings.Contains(err.Error(), "expected 3 segments") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestUsernameFromToken_BadFormat_OneSegment(t *testing.T) {
	_, err := usernameFromToken("not-a-jwt")
	if err == nil {
		t.Fatalf("expected error for malformed JWT")
	}
}

func TestUsernameFromToken_BadBase64(t *testing.T) {
	// Replace the payload with chars that are invalid even in URL-safe base64.
	bad := "header.!!!not-base64!!!.sig"
	_, err := usernameFromToken(bad)
	if err == nil {
		t.Fatalf("expected error for invalid base64 payload")
	}
	if !strings.Contains(err.Error(), "base64") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestUsernameFromToken_BadJSON(t *testing.T) {
	header := base64.RawURLEncoding.EncodeToString([]byte("{}"))
	payload := base64.RawURLEncoding.EncodeToString([]byte("not-json"))
	sig := base64.RawURLEncoding.EncodeToString([]byte("sig"))
	_, err := usernameFromToken(header + "." + payload + "." + sig)
	if err == nil {
		t.Fatalf("expected error for non-JSON payload")
	}
	if !strings.Contains(err.Error(), "parsing JWT claims") {
		t.Fatalf("unexpected error message: %v", err)
	}
}
