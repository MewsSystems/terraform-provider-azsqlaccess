# Test-server setup helper

One-shot Terraform module that stands up the long-lived Azure SQL + Postgres Flexible Server used by the acceptance suite. **CI does not run this module** — it expects the servers to already exist (see [`../README.md`](../README.md), prereq items 9 and 10).

The acceptance tests create + drop the per-test principals themselves; the servers are stage props.

## What it creates

- Azure SQL server `<prefix>-mssql-<run_id>` + database `<prefix>_db`, Entra-only auth, with the supplied UAMI attached for Graph lookups.
- Postgres Flexible Server `<prefix>-pg-<run_id>` + database `<prefix>_pg_db`, Entra-only auth.
- A single Entra principal (the one you pass via `entra_admin_object_id`) configured as the Entra admin on both servers.

~8 min to apply (Postgres dominates).

## Inputs

| Variable | Example |
|---|---|
| `subscription_id` | `00000000-0000-0000-0000-000000000000` |
| `resource_group_name` | `rg-azsqlaccess-acceptance-tests` |
| `mssql_server_identity_id` | `/subscriptions/.../providers/Microsoft.ManagedIdentity/userAssignedIdentities/...` |
| `entra_admin_object_id` | `00000000-0000-0000-0000-000000000000` |
| `run_id` | `acc` |
| `name_prefix` *(default `tfacc`)* | — |

> **About `entra_admin_object_id`.** This is the single Entra admin set on both servers (MSSQL allows only one — we keep Postgres in lockstep for simplicity). **Pass the object_id of whoever is going to run `terraform apply`:**
>
> - **Locally:** your own Entra user object_id (`az ad signed-in-user show --query id -o tsv`). This is what lets you connect to the servers from `az`/`psql`/SSMS afterwards.
> - **In CI/CD:** the CI managed identity's (or service principal's) object_id. This is what lets the CI runner connect during the acceptance suite.
>
> Re-apply the fixture from the *other* side when you switch contexts — applying locally with your user OID will overwrite CI's admin and vice versa. That's by design: this fixture provisions long-lived test servers, not a shared-admin setup.

## Outputs

Each one maps to a same-named repo Variable in the acceptance workflow — see [`../README.md`](../README.md).

## Apply / plan / destroy

```bash
cd tests/acceptance/fixtures
terraform init

terraform plan \
  -var="subscription_id=$AZURE_SUBSCRIPTION_ID" \
  -var="resource_group_name=$AZSQLACCESS_TEST_RG" \
  -var="run_id=acc" \
  -var="mssql_server_identity_id=$AZSQLACCESS_TEST_MSSQL_IDENTITY_ID" \
  -var="entra_admin_object_id=$AZSQLACCESS_TEST_ADMIN_OBJECT_ID"

terraform apply -auto-approve <same -var flags>
terraform destroy -auto-approve <same -var flags>
```

State is local — this module ships no `backend` block. Add your own if you want it shared.

`terraform destroy` polls the `azurerm_postgresql_flexible_server_active_directory_administrator` delete LRO, which has been observed to take tens of minutes. Be patient or finish cleanup from the portal.
