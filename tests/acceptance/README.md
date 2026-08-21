# Acceptance test prerequisites

The acceptance suite hits **real Azure infrastructure**. CI provisions nothing — it expects the resources below to exist and just runs `TF_ACC=1 go test`. The tests create + drop the per-test principals (`azsqlaccess_user`, `azsqlaccess_database_role_member`) themselves; the servers are long-lived stage props.

A maintainer helper for the one-time server setup lives in [`fixtures/`](./fixtures/README.md).

## Prerequisites

| # | Resource | Notes |
|---|---|---|
| 1 | **Azure subscription** | Hosts everything below. |
| 2 | **Resource group** | CI identity needs `Contributor` here (no broader scope). |
| 3 | **CI identity** (MI or App Reg + SP) | `Contributor` on the RG; Entra admin on the test servers (set once at server creation). |
| 4 | **Federated credential** *(recommended)* or **client secret** on the CI identity | Federated: trust `repo:<owner>/<repo>:environment:<env>`. Client secret: store as `AZURE_CLIENT_SECRET`. |
| 5 | **Test Entra user** | Real UPN. Never signs in. |
| 6 | **Test Entra security group** | Empty group is fine. |
| 7 | **Test Entra app reg + SP** | Keep separate from item 3. |
| 8 | **GitHub Environment** *(only if using federation)* | Optionally require maintainer approval. |
| 9 | **Azure SQL server + database** | Entra-only. A UAMI with the Entra `Directory Readers` role must be attached as `primary_user_assigned_identity_id` — Azure SQL needs it for Graph lookups during `CREATE USER ... FROM EXTERNAL PROVIDER`. The UAMI can be the CI identity itself if it's already a UAMI with that role. |
| 10 | **Postgres Flexible Server + database** | Entra-only. No extra MI/Graph setup — pgaadauth takes object IDs directly. |
| 11 | **Graph read on the CI identity** | `Group.Read.All` + `Application.Read.All` (or the `Directory Readers` role). Only `TestAccUser_{group,servicePrincipal}DeferredObjectID_postgres` need it — they resolve `object_id` through the `azuread` provider instead of hardcoding it, which is the only way to exercise an unknown `object_id` at validate time. Without it those two tests fail on the data-source read; the rest are unaffected. |

Items 9 and 10 can be provisioned with [`fixtures/`](./fixtures/README.md) — that's a one-shot apply, not a per-run dependency. The other prereqs you create by hand.

**Permissions the operator needs once:** subscription `Owner` / `User Access Administrator` (RBAC grants), tenant ability to create app regs + users + groups, and `Privileged Role Administrator` to grant Directory Readers to the UAMI in item 9.

## GitHub Actions config

**Secrets** (auto-redacted in logs):

| Secret | Example |
|---|---|
| `AZURE_TENANT_ID` | `aaaa...` |
| `AZURE_CLIENT_ID` | `bbbb...` |
| `AZURE_SUBSCRIPTION_ID` | `cccc...` |
| `AZSQLACCESS_TEST_GROUP_OBJECT_ID` | `dddd...` |
| `AZSQLACCESS_TEST_SP_OBJECT_ID` | `eeee...` |
| `AZURE_CLIENT_SECRET` *(only with client-secret auth)* | `<random>` |

**Variables**:

| Variable | Example |
|---|---|
| `AZSQLACCESS_TEST_MSSQL_SERVER` | `myserver.database.windows.net` |
| `AZSQLACCESS_TEST_DATABASE` | `mydb` |
| `AZSQLACCESS_TEST_POSTGRES_SERVER` | `myserver.postgres.database.azure.com` |
| `AZSQLACCESS_TEST_POSTGRES_DATABASE` | `mydb` |
| `AZSQLACCESS_TEST_USER_UPN` | `juan.perez@milanesa.com` |
| `AZSQLACCESS_TEST_GROUP_NAME` | `db.reader` |
| `AZSQLACCESS_TEST_SP_NAME` | `myapp-identity` |
| `AZSQLACCESS_TEST_POSTGRES_ADMIN_GROUP` *(optional)* | `db.reader` |

## Running in CI

Actions tab → **Acceptance Tests → Run workflow**. Wall time ~3–5 min.

Auth auto-selects: when `AZURE_CLIENT_SECRET` is unset (the default), the workflow exports `ARM_USE_OIDC=true` and the provider does the GitHub Actions OIDC federation exchange against the runner-injected `ACTIONS_ID_TOKEN_REQUEST_URL` / `_TOKEN` — no extra workflow step required. Set `AZURE_CLIENT_SECRET` and it falls back to client-secret auth.

`concurrency: { group: acceptance-tests }` queues runs — tests use stable principal names on the shared servers, so two concurrent runs would collide.

## Running locally

Same env vars as CI plus `az login`. Put them in a `.env` file at the repo root (gitignored) for convenience:

```bash
# .env
export AZSQLACCESS_TEST_MSSQL_SERVER=myserver.database.windows.net
export AZSQLACCESS_TEST_DATABASE=mydb
export AZSQLACCESS_TEST_POSTGRES_SERVER=myserver.postgres.database.azure.com
export AZSQLACCESS_TEST_POSTGRES_DATABASE=mydb
export AZSQLACCESS_TEST_USER_UPN=<test UPN — see warning below>
export AZSQLACCESS_TEST_GROUP_NAME=<test group display name>
export AZSQLACCESS_TEST_GROUP_OBJECT_ID=<test group OID>
export AZSQLACCESS_TEST_SP_NAME=<test SP display name>
export AZSQLACCESS_TEST_SP_OBJECT_ID=<test SP OID>

# Optional — enables the group-login test, see below.
export AZSQLACCESS_TEST_POSTGRES_ADMIN_GROUP=<Entra group that admins the PG server>
```

Then:

```bash
az login
az account set --subscription "<sub>"
source .env

make testacc-only                                                      # acceptance tests only
make testacc                                                           # unit + acceptance
TF_ACC=1 go test -v -run TestAccUser_user_mssql ./internal/provider/...  # one test
```

Local auth comes from the Azure CLI session — the provider's ambient chain tries `az` first. OIDC is CI-only; no setup needed locally.

If you need to (re)build the test servers, use [`fixtures/`](./fixtures/README.md) — `terraform plan` / `apply` / `destroy` all run locally with the same `-var` flags.

### ⚠️ `AZSQLACCESS_TEST_USER_UPN` cannot match the Postgres Entra admin

Whoever sets up the Postgres server (via `fixtures/` or by hand) becomes its Entra admin, and `pgaadauth` reserves the admin's UPN as a role name. If you then point `AZSQLACCESS_TEST_USER_UPN` at the same UPN, `TestAccUser_user_postgres` fails with "role already exists". Use a different real Entra user. Group and SP tests are unaffected.

In CI this doesn't trigger because the Postgres admin is the CI SP and the test user is a real human UPN.

### Optional: `AZSQLACCESS_TEST_POSTGRES_ADMIN_GROUP` (group login)

`TestAccUser_user_postgres_viaGroupLogin` covers the provider's `login_username`: connecting as an Entra group's role instead of as the caller. It skips unless this variable is set, because it needs a setup the other tests don't — a group that is **both** an Entra administrator on the Postgres server **and** contains the identity running the test.

Set it to that group's display name. The caller still presents its own token; only the role it assumes changes. If the variable names a group that isn't a server administrator, the test fails at connect with `FATAL: password authentication failed`.

### Iteration tips

- `TF_LOG=DEBUG` for full Terraform trace; `TF_LOG_PROVIDER=DEBUG` for just provider logs.
- Without `az login`, set `AZURE_CLIENT_ID` / `AZURE_TENANT_ID` / `AZURE_CLIENT_SECRET` for SP auth — the provider's ambient chain (Azure CLI → Workload Identity → Managed Identity) picks up whatever's configured.

## External contributors

Fork, configure the same secrets + variables against your own sub, trigger the workflow from your fork, link the green run in the PR.
