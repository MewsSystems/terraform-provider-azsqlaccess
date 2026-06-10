# Define a user we'll grant a role to. The role member references this user's
# `name` attribute, which lets Terraform infer the create-order automatically —
# no explicit depends_on needed.
resource "azsqlaccess_user" "reader" {
  server   = "myserver.database.windows.net"
  database = "mydb"
  type     = "user"
  name     = "juan.perez@milanesa.com"
}

# Grant the built-in `db_datareader` role on Azure SQL. On PostgreSQL Flexible
# Server use `pg_read_all_data` and target the postgres provider alias.
resource "azsqlaccess_database_role_member" "reader" {
  server   = "myserver.database.windows.net"
  database = "mydb"
  role     = "db_datareader"
  member   = azsqlaccess_user.reader.name
}
