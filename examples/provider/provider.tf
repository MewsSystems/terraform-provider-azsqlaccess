terraform {
  required_providers {
    azsqlaccess = {
      source = "MewsSystems/azsqlaccess"
    }
  }
}

# Azure SQL — engine = "mssql"
provider "azsqlaccess" {
  engine = "mssql"

  # Optional — falls back to AZURE_TENANT_ID / AZURE_CLIENT_ID / AZURE_CLIENT_SECRET.
  # Omit entirely to use the ambient credential chain (Azure CLI, then Workload Identity, then Managed Identity).
  # tenant_id     = "00000000-0000-0000-0000-000000000000"
  # client_id     = "00000000-0000-0000-0000-000000000000"
  # client_secret = "super-secret"
}

# Azure PostgreSQL Flexible Server — alias is required when configuring both engines
# in the same root module. Resources opt in via `provider = azsqlaccess.postgres`.
provider "azsqlaccess" {
  alias  = "postgres"
  engine = "postgres"

  # Optional, PostgreSQL only. Set this when the caller is an administrator only by
  # way of Entra group membership: PostgreSQL Flexible Server does not expand groups
  # server-side, so the connection must ask for the group's own role name while still
  # presenting the caller's token. Omit to connect as the caller itself.
  # login_username = "db.reader"
}
