# CLAUDE.md

Operational notes for working in this repository. The README is the user-facing reference — read it first for usage. This file is what Claude (or a new contributor) needs to be effective on day one.

## Project at a glance

- **What:** Terraform provider that manages contained database users and role memberships in **Azure SQL** and **Azure PostgreSQL Flexible Server** via **Entra ID** (Azure AD).
- **Not supported:** SQL Server username/password auth. Entra-only by design.
- **Resources:** `azsqlaccess_user`, `azsqlaccess_database_role_member`.
- **Provider source:** `MewsSystems/azsqlaccess` → `registry.terraform.io/MewsSystems/azsqlaccess` (publication pending). Namespace matches the GitHub org that owns the repo.
- **Module path:** `github.com/mews/terraform-provider-azsqlaccess`.
- **Framework:** [terraform-plugin-framework](https://developer.hashicorp.com/terraform/plugin/framework) — **not** the deprecated SDKv2.
- **Go:** see `go.mod` (currently 1.25.7).

## Status

| Phase | Description | Status |
|---|---|---|
| 1-4 | Provider scaffold, MSSQL connector, resources, `Configure()` | Done |
| 5-6 | PostgreSQL connector + same resources working on both engines | Done |
| 7 | Acceptance tests, docs, examples | Docs/examples done; acceptance tests still TODO |
| 8 | Registry publication: GPG, App install, tag v0.1.0 | Infra wired; GPG + App install pending |
| 9 | `data_azsqlaccess_user` data source | Not started |

The Confluence "Architecture & Status" page (INFRA space, ID 1780613391) is the canonical roadmap. Update it when Phase 7/8 progress.

## Architecture (the most important part)

Three layers with strict top-down dependency:

```
internal/provider/   ← Plugin Framework shell; engine selection + ConnectorFactory injection
internal/resources/  ← Terraform resources; engine-agnostic
internal/database/   ← DatabaseConnector interface + per-engine implementations
  ├── mssql/         ← go-mssqldb NewAccessTokenConnector + shared azcore.TokenCredential
  └── postgres/      ← pgx/v5 + shared azcore.TokenCredential via BeforeConnect
```

**Hard rule:** code under `internal/resources/` may import `internal/database` (the interface) but **must never** import `internal/database/mssql` or `internal/database/postgres`. The engine is invisible to the resource layer. Adding a new engine = new package + one `case` in `provider.Configure()`. Zero changes anywhere else.

The contract is the `DatabaseConnector` interface in `internal/database/connector.go`. Every CRUD op on every resource goes through it. The `database` is encoded in the DSN at connect time — it is **not** a method parameter; each connector is scoped to one (server, database) pair.

## Key conventions specific to this project

- **Copyright header on every Go file:**
  ```go
  // Copyright (c) Mews Systems
  // SPDX-License-Identifier: MPL-2.0
  ```
- **Compile-time interface assertions** at the top of every resource:
  ```go
  var _ resource.Resource                = &UserResource{}
  var _ resource.ResourceWithConfigure   = &UserResource{}
  var _ resource.ResourceWithImportState = &UserResource{}
  ```
- **`MarkdownDescription`** (not `Description`) on every schema field — these flow into the registry's rendered docs and the VS Code extension. Treat them as user-facing copy.
- **Cross-attribute validation** lives in `ValidateConfig` (e.g. `type=user` forbids `object_id`). Per-attribute validators (`validators.StringOneOf`, `validators.StringUUID`) live in `internal/validators/`.
- **Plan modifiers in use:**
  - `RequiresReplace()` — for immutable attrs (`server`, `database`, `type`, `name`, `object_id`).
  - `UseStateForUnknown()` — for computed attrs (`id`, `principal_id`, `default_schema`) to avoid spurious plan churn.
- **Import ID format** is documented in the import-handler comment block on each resource and must match `Create()`'s ID assembly.
  - `azsqlaccess_user`: `server/database/user/upn` or `server/database/{group|service_principal}/name/object_id`
  - `azsqlaccess_database_role_member`: `server/database/role/member`
  - `object_id` is in the import ID for group/SP because it cannot be read back from the database (MSSQL stores SIDs in mixed-endian byte order; PostgreSQL doesn't store it at all).

## No personal or company-specific identifiers anywhere

**Hard rule. No exceptions.** This is a public-bound Terraform provider — `registry.terraform.io/MewsSystems/azsqlaccess`. Anything in source code, schema `MarkdownDescription` strings, error messages, examples, generated docs, README, tests, or test fixtures gets indexed by the registry and search engines, and harvested by phishing/spam scrapers. **Do not include any of the following anywhere in this repository:**

- Real UPNs or email addresses (e.g. anything `@mews.com` or any other real domain belonging to a person)
- Real employee names or usernames
- Real Entra group display names (especially internal naming conventions like `eng.<team>.<sub>.<x>`)
- Real managed identity, service principal, or app registration names
- Real Entra Object IDs / tenant IDs / subscription IDs / any UUID belonging to a real Azure object
- Real server hostnames (production, staging, anyone's dev server)
- Real database names, role names that match internal conventions, or schema names

This rule applies equally to: `.go` files (including comments and `MarkdownDescription`), `.tf` examples, `.sh` import scripts, README, CHANGELOG, test files, test data fixtures, and any future docs. `tfplugindocs` mirrors `MarkdownDescription` and example files verbatim into public registry pages — there is no "internal-only" surface in this repo.

Use **only** these placeholders:

| Slot | Value |
|---|---|
| UPN | `juan.perez@milanesa.com` |
| Group display name | `db.reader` |
| Service principal / managed identity name | `myapp-identity` |
| Object IDs (and any UUID placeholder) | `00000000-0000-0000-0000-000000000000` |
| Server (MSSQL) | `myserver.database.windows.net` |
| Server (PostgreSQL) | `myserver.postgres.database.azure.com` |
| Database | `mydb` |

**Before opening a PR**, grep the diff for `mews.com`, real employee names, the company's group naming patterns, and the strings `tenant`/`subscription`/`object` near a UUID to verify nothing real slipped in. If you need a placeholder for a slot not in the table above, add it to the table — never invent a new "realistic-looking" alternative inline.

## Where things live

```
main.go                                Provider entrypoint; sets registry address
internal/provider/provider.go          Engine switch; builds factory; injects into resources
internal/resources/<r>/resource.go     CRUD + Schema + ValidateConfig + ImportState
internal/resources/<r>/model.go        tfsdk-tagged struct mirroring the schema
internal/database/connector.go         DatabaseConnector + ConnectorFactory interfaces
internal/database/credential.go        BuildEntraCredential: SP → ARM_USE_OIDC → ChainedTokenCredential(AzureCLI, WIC, MIC)
internal/database/models.go            User / RoleMember structs
internal/database/{mssql,postgres}/    Engine implementations (share one azcore.TokenCredential)
internal/database/retry.go             Shared backoff helper (cenkalti/backoff/v4)
internal/validators/validators.go      StringOneOf, StringUUID
examples/provider/provider.tf          → docs/index.md
examples/resources/<r>/resource.tf     → docs/resources/<r>.md "Example Usage"
examples/resources/<r>/import.sh       → docs/resources/<r>.md "Import"
docs/                                  GENERATED — never hand-edit
tools/tools.go                         tfplugindocs entry
.goreleaser.yml                        Cross-compile, ZIP, SHA256SUMS, GPG checksum sign
.github/workflows/release-please.yml   On main: release-please opens/merges the release PR → tags v* + creates the Release (no publish)
.github/workflows/release.yml          workflow_dispatch: goreleaser publish (GPG sign + upload) against a chosen v* tag
.github/workflows/test.yml             Build, lint, generate-diff check, unit tests on every PR
.github/workflows/acceptance.yml       workflow_dispatch-only acceptance tests against real Azure
```

## Common commands

```bash
make build          # go build -v ./...
make install        # go install -v ./...
make lint           # golangci-lint run
make fmt            # gofmt -s -w -e .
make test           # unit tests (no Azure required)
make testacc        # TF_ACC=1 acceptance tests (needs Azure subscription)
make generate       # tfplugindocs generate — regenerates docs/ from MarkdownDescription + examples/
```

The CI `generate` job runs `make generate` then does `git diff --exit-code`. **Always commit regenerated `docs/` after schema or example changes** or CI will fail.

## Best practices for working here

### Default to HashiCorp / Terraform Plugin Framework conventions

**When in doubt, do what HashiCorp's reference repos do.** Specifically: [`terraform-provider-scaffolding-framework`](https://github.com/hashicorp/terraform-provider-scaffolding-framework), [`terraform-plugin-framework`](https://github.com/hashicorp/terraform-plugin-framework), and the existing scaffolding files this repo was generated from (`.goreleaser.yml`, `.github/workflows/`, `terraform-registry-manifest.json`, `META.d/`). These represent the patterns the registry expects and the patterns other Azure-shop providers will recognise.

Practical applications:

- **Workflows.** Use the `actions/setup-go` invocation HashiCorp ships in scaffolding — `go-version-file: 'go.mod' + cache: true`, no `check-latest`, no manual `GOTOOLCHAIN`. Pin-exact is the convention; it surfaces directive drift loudly. Don't "improve" it without a concrete reason this repo specifically needs to diverge.
- **Schema patterns.** Plan modifiers, validators, schema field naming (`MarkdownDescription`, `RequiresReplace`, `UseStateForUnknown`) — copy the patterns from Plugin Framework examples and `terraform-provider-scaffolding-framework`'s example resource. Don't invent.
- **Release pipeline.** `goreleaser` config, GPG signing, the `release.yml` goreleaser job shape, `terraform-registry-manifest.json` — all from HashiCorp's published templates. Anything that affects what the registry consumes must follow them verbatim.
- **Linter config.** `.golangci.yml` mirrors HashiCorp's recommended set for providers (the v2 config inherited from scaffolding). Add to it (e.g. `goheader`); don't replace its base.
- **Test conventions.** Acceptance tests live in `internal/provider/*_acceptance_test.go`, gated by `TF_ACC=1`, using `providerserver.NewProtocol6WithError` and the `testAccProtoV6ProviderFactories` shape defined in `provider_test.go`. PR CI runs unit tests only (no `TF_ACC`); acceptance tests run via `workflow_dispatch` on `acceptance.yml` against long-lived Azure SQL + Postgres servers documented as prereqs in `tests/acceptance/README.md` — CI provisions nothing.

When a HashiCorp/Plugin-Framework convention conflicts with a third-party suggestion (Copilot, AI reviewer, blog post), the HashiCorp convention wins unless there's a documented project-specific reason to deviate. Document that reason in CLAUDE.md or a code comment if you ever do.

### Plugin Framework

- **Use `MarkdownDescription`, not `Description`.** Markdown is rendered in the registry; plain text is not.
- **`RemoveResource(ctx)` in `Read`** when the underlying object is gone. This triggers re-create on next plan instead of erroring.
- **`Optional+Computed`** for fields the engine defaults (e.g. `default_schema` defaults to `dbo`/`public`). Use `UseStateForUnknown()` so refreshes don't churn the plan.
- **Cross-attribute rules → `ValidateConfig`.** Per-attribute validators can't see siblings. Skip validation when `IsUnknown()` — the framework re-runs after planning resolves the value.
- **Always wrap errors with context** (`server`, `database`, principal name). The user is debugging from `terraform apply` output, not a stack trace.

### SQL safety

- **Parameter binding** for values: `@p1` (MSSQL), `$1` (pgx).
- **Identifier quoting** for things that can't be parameter-bound (role names, user names, schema names): `[name]` for MSSQL, `"name"` for PostgreSQL. Always go through the engine's quoting helpers — never string-concatenate into DDL.
- **Pre-existence check** before `CreateRoleMember` to prevent two resources silently claiming the same underlying grant. The fix path is `terraform import`, surfaced in the error message.

### Auth

- **One credential, two engines.** `database.BuildEntraCredential` is called once in `provider.Configure()` and the resulting `azcore.TokenCredential` is shared with both factories. Precedence: explicit SP (`tenant_id` + `client_id` + `client_secret`) → `ARM_USE_OIDC=true` (GitHub Actions OIDC, `ARM_OIDC_TOKEN`, or `ARM_OIDC_TOKEN_FILE_PATH`) → explicit `ChainedTokenCredential` of Azure CLI → Workload Identity → Managed Identity. Interactive flows (`InteractiveBrowserCredential`, `VisualStudioCodeCredential`, `AzureDeveloperCLICredential`) are intentionally excluded — Wiz WS-GO-01412 flags `DefaultAzureCredential` for that reason.
- **GitHub Actions OIDC parity with `azurerm`/`azuread`/`azapi`.** Setting `ARM_USE_OIDC=true` + the standard `ARM_*` triple is enough — the provider performs the federation exchange via `azidentity.NewClientAssertionCredential` and the runner-injected `ACTIONS_ID_TOKEN_REQUEST_URL/TOKEN`. No extra workflow step needed.
- **Never use `go-autorest/adal`** — Microsoft deprecated it in 2023. Always `github.com/Azure/azure-sdk-for-go/sdk/azidentity`.
- **Token plumbing.** MSSQL uses `mssql.NewAccessTokenConnector` with a token-provider callback (scope `https://database.windows.net/.default`). PostgreSQL injects the token as the connection password via pgx's `BeforeConnect` (scope `https://ossrdbms-aad.database.windows.net/.default`). Both call the same `azcore.TokenCredential` — keep it that way.

### Connection pooling

- **One pool per `(server, database)`** key, cached on the factory under a `sync.Mutex`. `Close()` is a no-op — pools persist for the whole Terraform run.
- The PostgreSQL factory **also caches the system `postgres` database pool** because `pgaadauth_create_principal` runs in that database, not the target one.

### Retry

- **Only retry on transient errors.** This codebase retries deadlocks only:
  - MSSQL: error number `1205`.
  - PostgreSQL: SQLSTATE `40P01`.
- Auth, syntax, and missing-object errors are permanent — surface them immediately. `cenkalti/backoff/v4` with bounded attempts (currently 4).

### Examples and docs

- **`docs/` is generated.** Never hand-edit. Run `make generate` and commit the result.
- **Examples are the source of truth for the rendered docs.** Each file has a specific role:
  - `examples/provider/provider.tf` → registry index page.
  - `examples/resources/<full-resource-name>/resource.tf` → "Example Usage" section.
  - `examples/resources/<full-resource-name>/import.sh` → "Import" section.
- **Examples must be valid HCL** — `tfplugindocs` formats them with `terraform fmt` during generation.

### Releasing

- Versioning and publishing are split. **release-please** (off Conventional Commits — never tag by hand) opens/merges the release PR, which tags `v*` and creates the GitHub Release. Publishing is **manual for now**: dispatch `release.yml` against the `v*` tag to import GPG + run goreleaser and upload signed artifacts. The Terraform Registry then polls and indexes within minutes. See [CONTRIBUTING.md](CONTRIBUTING.md).
- The Registry **rejects unsigned releases**. GPG signing is mandatory.
- Required GitHub secrets: `GPG_PRIVATE_KEY`, `PASSPHRASE`. Public key registered at `registry.terraform.io`. Terraform Registry GitHub App must be installed on the org.
- **Never** force-push tags or amend a published commit on this repo — registry indexers cache by commit SHA.

### Stay consistent with CI/CD

**Every code or config change must be cross-checked against the CI workflows in `.github/workflows/` before being declared done.** A change that builds and tests locally but breaks CI is a defect — local Go often has `GOTOOLCHAIN=auto` and a newer toolchain installed, while CI runs with `GOTOOLCHAIN=local` and exactly the version the root `go.mod` declares. The asymmetry hides bugs.

Specific traps to remember in this repo:

- **Go directive coupling.** The CI `setup-go` step uses `go-version-file: 'go.mod'` (root). `setup-go` v6 also implicitly sets `GOTOOLCHAIN: local` on the runner, so Go refuses to auto-fetch a newer toolchain at build time. If `tools/go.mod`'s `go` directive ever exceeds the root's, the `generate` job fails immediately when it enters `tools/`. **Whenever `tools/go.mod` bumps its `go` directive, bump the root's to match in the same commit.** Confirm with `grep '^go ' go.mod tools/go.mod`.
- **No `check-latest` on `setup-go`** — pin-exact is intentional. Following HashiCorp's `terraform-provider-scaffolding` convention, the workflows install the exact version in `go.mod`. The trade-off: contributors must bump the directive themselves to pick up new Go patches, but cross-module directive misalignment surfaces loudly in CI instead of being hidden by a "latest patch" auto-upgrade.
- **`make generate` is gated by CI.** The `generate` job runs `make generate` then `git diff --exit-code`. Any change that affects schema `MarkdownDescription`, examples, or `tfplugindocs` itself must be followed by `make generate` + committing the regenerated `docs/`.
- **Linter version drift.** CI uses the latest `golangci-lint` v2 release (per `golangci/golangci-lint-action`). Locally, prefer `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run` over a system-installed binary so you exercise the same rules.
- **Reproduce CI behaviour locally before pushing** when in doubt:
  ```bash
  GOTOOLCHAIN=local go build ./...
  GOTOOLCHAIN=local go test ./...
  GOTOOLCHAIN=local make generate
  ```
  This refuses any toolchain auto-fetch, exactly like CI does. Errors here predict errors there.

### Updating Go dependencies

Two-module repo (`./go.mod` for the runtime, `./tools/go.mod` for `tfplugindocs` under a `//go:build generate` tag). Recipe lives in [README.md → Updating dependencies](README.md#updating-dependencies). Two non-obvious traps that have already burned us:

- **`go get -u ./...` does not update `tools/go.mod`'s direct dep.** `tfplugindocs` is a `main` package; Go's `./...` resolver refuses to traverse it (`"... is a program, not an importable package"`). Use `go get -u github.com/hashicorp/terraform-plugin-docs` explicitly inside `tools/`.
- **`go get -u` can raise `tools/go.mod`'s `go` directive without touching the root's.** Always re-check `grep '^go ' go.mod tools/go.mod` after a deps cycle and bump the root with `go mod edit -go=<x.y.z>` if `tools/` moved past it. See the "Go directive coupling" trap above for why.

Always finish a deps cycle with `GOTOOLCHAIN=local make generate && go test ./...` before pushing.

## Things to avoid

- **Don't import `internal/database/mssql` or `internal/database/postgres`** from any package other than `internal/provider/provider.go`. Resources speak only to the interface in `internal/database`.
- **Don't add resource attributes without `MarkdownDescription`.** They become invisible columns on the registry page.
- **Don't put real internal identifiers in source files.** They are mirrored verbatim into public docs by `tfplugindocs`. Use the placeholder table above.
- **Don't hand-edit anything under `docs/`.** It will be overwritten on the next `make generate`.
- **Don't catch errors with bare `error`.** Wrap with `fmt.Errorf("...: %w", err)` and include `server`/`database`/principal name in the message.
- **Don't add backwards-compatibility shims** before v1.0. The breaking-change budget is wide open until then; use it to keep the surface clean.
- **Don't rely on reading `object_id` from the database.** MSSQL byte-swaps it in the SID column; PostgreSQL doesn't store it. It comes from config or the import ID — nowhere else.

## When in doubt

- Read the resource's `Schema()` and `ImportState()` methods first — they encode most invariants.
- The `DatabaseConnector` interface is the contract. If a change requires touching it, it requires touching every engine implementation.
- The README is canonical for end-user behavior. The Confluence page is canonical for project status. This file is canonical for "how do I work in this repo without breaking things."
