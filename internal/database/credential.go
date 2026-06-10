// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

// oidcExchangeAudience is the audience Entra expects for federated client assertions.
const oidcExchangeAudience = "api://AzureADTokenExchange"

// oidcFetchTimeout bounds a single OIDC ID-token fetch.
const oidcFetchTimeout = 30 * time.Second

// BuildEntraCredential constructs the Entra credential the provider uses for
// Azure SQL and Postgres Flexible Server, with precedence matching
// azurerm/azuread/azapi so pipelines wired for those providers work unchanged:
//
//  1. Explicit service principal: tenant_id + client_id + client_secret.
//  2. GitHub Actions OIDC: ARM_USE_OIDC=true plus ACTIONS_ID_TOKEN_REQUEST_URL/TOKEN.
//  3. Explicit federated assertion: ARM_USE_OIDC=true plus ARM_OIDC_TOKEN or
//     ARM_OIDC_TOKEN_FILE_PATH (for non-GitHub CIs).
//  4. Ambient fallback chain: Azure CLI → workload identity → managed identity.
//
// (4) is an explicit ChainedTokenCredential, not DefaultAzureCredential. DAC
// transparently extends to interactive flows (InteractiveBrowserCredential,
// VisualStudioCodeCredential, AzureDeveloperCLICredential) which are not
// appropriate for a non-interactive provider and broaden the trust surface.
func BuildEntraCredential(tenantID, clientID, clientSecret string) (azcore.TokenCredential, error) {
	if tenantID != "" && clientID != "" && clientSecret != "" {
		cred, err := azidentity.NewClientSecretCredential(tenantID, clientID, clientSecret, nil)
		if err != nil {
			return nil, fmt.Errorf("client secret credential: %w", err)
		}
		return cred, nil
	}

	if oidcEnabled() {
		tID := firstNonEmpty(tenantID, os.Getenv("ARM_TENANT_ID"))
		cID := firstNonEmpty(clientID, os.Getenv("ARM_CLIENT_ID"))
		if tID == "" || cID == "" {
			return nil, errors.New("ARM_USE_OIDC=true but tenant_id/client_id are not set " +
				"(supply via provider block or ARM_TENANT_ID/ARM_CLIENT_ID env vars)")
		}
		cred, err := newOIDCCredential(tID, cID)
		if err != nil {
			return nil, fmt.Errorf("OIDC credential: %w", err)
		}
		return cred, nil
	}

	return newAmbientCredential()
}

// newAmbientCredential builds the non-interactive fallback chain. CLI is
// tried first so local dev with `az login` succeeds in <100ms; on Azure
// compute it fast-fails (no `az` binary) and the chain falls through to
// workload/managed identity. Putting ManagedIdentity first instead caused
// 30s hangs on laptops where the IMDS endpoint isn't reachable.
func newAmbientCredential() (azcore.TokenCredential, error) {
	var sources []azcore.TokenCredential

	if cli, err := azidentity.NewAzureCLICredential(nil); err == nil {
		sources = append(sources, cli)
	}
	if wic, err := azidentity.NewWorkloadIdentityCredential(nil); err == nil {
		sources = append(sources, wic)
	}
	if mic, err := azidentity.NewManagedIdentityCredential(nil); err == nil {
		sources = append(sources, mic)
	}

	if len(sources) == 0 {
		return nil, errors.New("no Entra credential source available: " +
			"set tenant_id/client_id/client_secret, enable ARM_USE_OIDC, " +
			"or sign in with `az login`")
	}

	cred, err := azidentity.NewChainedTokenCredential(sources, nil)
	if err != nil {
		return nil, fmt.Errorf("chained token credential: %w", err)
	}
	return cred, nil
}

// oidcEnabled mirrors azurerm's parsing of ARM_USE_OIDC so the env-var UX is identical.
func oidcEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("ARM_USE_OIDC"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// newOIDCCredential picks the assertion source based on which ARM_OIDC_* /
// ACTIONS_ID_TOKEN_REQUEST_* env vars are populated.
func newOIDCCredential(tenantID, clientID string) (azcore.TokenCredential, error) {
	switch {
	case os.Getenv("ARM_OIDC_TOKEN") != "":
		token := os.Getenv("ARM_OIDC_TOKEN")
		return azidentity.NewClientAssertionCredential(tenantID, clientID,
			func(_ context.Context) (string, error) { return token, nil }, nil)

	case os.Getenv("ARM_OIDC_TOKEN_FILE_PATH") != "":
		path := os.Getenv("ARM_OIDC_TOKEN_FILE_PATH")
		return azidentity.NewClientAssertionCredential(tenantID, clientID,
			func(_ context.Context) (string, error) { return readTokenFile(path) }, nil)

	case os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL") != "" && os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN") != "":
		reqURL := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL")
		reqToken := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
		return azidentity.NewClientAssertionCredential(tenantID, clientID,
			func(ctx context.Context) (string, error) {
				return fetchGitHubOIDCToken(ctx, http.DefaultClient, reqURL, reqToken)
			}, nil)

	default:
		return nil, errors.New("ARM_USE_OIDC=true but no OIDC token source is configured " +
			"(set ARM_OIDC_TOKEN, ARM_OIDC_TOKEN_FILE_PATH, or run inside GitHub Actions)")
	}
}

// fetchGitHubOIDCToken calls the runner-injected OIDC endpoint with audience
// api://AzureADTokenExchange. HTTPS-only — refusing other schemes prevents an
// attacker-controlled URL from harvesting the request bearer.
func fetchGitHubOIDCToken(ctx context.Context, client *http.Client, reqURL, reqToken string) (string, error) {
	u, err := url.Parse(reqURL)
	if err != nil {
		return "", fmt.Errorf("parsing OIDC request URL: %w", err)
	}
	if u.Scheme != "https" {
		return "", fmt.Errorf("OIDC request URL must be https, got scheme %q", u.Scheme)
	}
	q := u.Query()
	q.Set("audience", oidcExchangeAudience)
	u.RawQuery = q.Encode()

	ctx, cancel := context.WithTimeout(ctx, oidcFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", fmt.Errorf("building OIDC request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+reqToken)
	req.Header.Set("Accept", "application/json; api-version=2.0")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling OIDC endpoint: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("reading OIDC response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OIDC endpoint returned HTTP %d: %s",
			resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("decoding OIDC response: %w", err)
	}
	if payload.Value == "" {
		return "", errors.New("OIDC response missing token value")
	}
	return payload.Value, nil
}

func readTokenFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading OIDC token file: %w", err)
	}
	token := strings.TrimSpace(string(b))
	if token == "" {
		return "", fmt.Errorf("OIDC token file %s is empty", path)
	}
	return token, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
