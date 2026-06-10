output "mssql_fqdn" {
  description = "Set AZSQLACCESS_TEST_MSSQL_SERVER to this value."
  value       = azurerm_mssql_server.test.fully_qualified_domain_name
}

output "mssql_database" {
  description = "Set AZSQLACCESS_TEST_DATABASE to this value."
  value       = azurerm_mssql_database.test.name
}

output "postgres_fqdn" {
  description = "Set AZSQLACCESS_TEST_POSTGRES_SERVER to this value."
  value       = azurerm_postgresql_flexible_server.test.fqdn
}

output "postgres_database" {
  description = "Set AZSQLACCESS_TEST_POSTGRES_DATABASE to this value."
  value       = azurerm_postgresql_flexible_server_database.test.name
}
