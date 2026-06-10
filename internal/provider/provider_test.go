// Copyright (c) Mews Systems
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// testAccProtoV6ProviderFactories spins up the provider in-process for
// acceptance tests. The map key ("azsqlaccess") must match the provider
// source address used inside test HCL configs.
//
// Unit tests live alongside the framework code (provider_unit_test.go,
// resource_*_test.go) — those are not gated by TF_ACC and run by default.
// Anything in a `TestAcc*`-named test reaches real Azure via the standard
// resource.Test() flow.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"azsqlaccess": providerserver.NewProtocol6WithError(New("test")()),
}

// testAccPreCheck runs at the start of every acceptance test. Fails fast if
// the operator hasn't exported the test-specific env vars — clearer than
// surfacing the failure deep inside a SQL call.
//
// Azure auth env vars are intentionally NOT enforced here. database.BuildEntraCredential
// resolves the right credential from whichever of these is configured:
//   - service principal (AZURE_*/ARM_* tenant/client/secret triple)
//   - GitHub Actions OIDC (ARM_USE_OIDC=true; runner-injected token)
//   - explicit OIDC token / token file (ARM_OIDC_TOKEN, ARM_OIDC_TOKEN_FILE_PATH)
//   - ambient ChainedTokenCredential (`az login`, Workload Identity, Managed Identity)
func testAccPreCheck(t *testing.T) {
	t.Helper()
	required := []string{
		"AZSQLACCESS_TEST_MSSQL_SERVER",
		"AZSQLACCESS_TEST_DATABASE",
		"AZSQLACCESS_TEST_POSTGRES_SERVER",
		"AZSQLACCESS_TEST_POSTGRES_DATABASE",
		"AZSQLACCESS_TEST_USER_UPN",
		"AZSQLACCESS_TEST_GROUP_NAME",
		"AZSQLACCESS_TEST_GROUP_OBJECT_ID",
		"AZSQLACCESS_TEST_SP_NAME",
		"AZSQLACCESS_TEST_SP_OBJECT_ID",
	}
	for _, name := range required {
		if os.Getenv(name) == "" {
			t.Fatalf("acceptance test requires env var %s to be set (see tests/acceptance/README.md)", name)
		}
	}
}
