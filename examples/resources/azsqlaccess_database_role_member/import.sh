# server/database/role/member
terraform import azsqlaccess_database_role_member.mssql_reader \
  "myserver.database.windows.net/mydb/db_datareader/juan.perez@milanesa.com"

terraform import azsqlaccess_database_role_member.postgres_reader \
  "myserver.postgres.database.azure.com/mydb/pg_read_all_data/db.reader"
