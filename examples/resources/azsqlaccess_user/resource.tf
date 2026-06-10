# Member user — name is the UPN, object_id must NOT be set.
resource "azsqlaccess_user" "user" {
  server   = "myserver.database.windows.net"
  database = "mydb"
  type     = "user"
  name     = "juan.perez@milanesa.com"
}

# Security or Microsoft 365 group — name is the display name; object_id is required
# because multiple groups in the same tenant can share a display name.
resource "azsqlaccess_user" "group" {
  server    = "myserver.database.windows.net"
  database  = "mydb"
  type      = "group"
  name      = "db.reader"
  object_id = "00000000-0000-0000-0000-000000000000"
}

# Service principal or managed identity — name is the Azure resource display name;
# object_id is required.
resource "azsqlaccess_user" "managed_identity" {
  server    = "myserver.database.windows.net"
  database  = "mydb"
  type      = "service_principal"
  name      = "myapp-identity"
  object_id = "00000000-0000-0000-0000-000000000000"
}

# Same resource shape on PostgreSQL Flexible Server — only `server` and the provider
# alias change. The `type` / `name` / `object_id` semantics are identical.
resource "azsqlaccess_user" "pg_group" {
  provider  = azsqlaccess.postgres
  server    = "myserver.postgres.database.azure.com"
  database  = "mydb"
  type      = "group"
  name      = "db.reader"
  object_id = "00000000-0000-0000-0000-000000000000"
}
