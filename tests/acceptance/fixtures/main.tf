data "azurerm_resource_group" "test" {
  name = var.resource_group_name
}

data "azuread_client_config" "current" {}

locals {
  common_tags = {
    purpose    = "acceptance-tests"
    managed_by = "terraform"
    run_id     = var.run_id
    created_at = timestamp()
  }
}

# Resolve the admin OID's directory type once, then look up only the matching
# object kind to derive `principal_name` for Postgres (UPN / appid / display name).
data "azuread_directory_object" "admin" {
  object_id = var.entra_admin_object_id
}

data "azuread_user" "admin" {
  count     = data.azuread_directory_object.admin.type == "User" ? 1 : 0
  object_id = var.entra_admin_object_id
}

data "azuread_service_principal" "admin" {
  count     = data.azuread_directory_object.admin.type == "ServicePrincipal" ? 1 : 0
  object_id = var.entra_admin_object_id
}

data "azuread_group" "admin" {
  count     = data.azuread_directory_object.admin.type == "Group" ? 1 : 0
  object_id = var.entra_admin_object_id
}

locals {
  admin_principal_name = coalesce(
    try(data.azuread_user.admin[0].user_principal_name, null),
    try(data.azuread_service_principal.admin[0].client_id, null),
    try(data.azuread_group.admin[0].display_name, null),
  )
}

resource "azurerm_mssql_server" "test" {
  name                = "${var.name_prefix}-mssql-${var.run_id}"
  resource_group_name = data.azurerm_resource_group.test.name
  location            = data.azurerm_resource_group.test.location
  version             = "12.0"

  azuread_administrator {
    azuread_authentication_only = true
    login_username              = "tfacc-admin"
    object_id                   = var.entra_admin_object_id
  }

  # Azure SQL calls Graph with the *server's* identity (not the connecting
  # client's) during CREATE USER ... FROM EXTERNAL PROVIDER. The attached MI
  # must have Directory Readers — see tests/acceptance/README.md.
  identity {
    type         = "UserAssigned"
    identity_ids = [var.mssql_server_identity_id]
  }
  primary_user_assigned_identity_id = var.mssql_server_identity_id

  tags = local.common_tags
}

# Test-only: open firewall. Server has no data and Entra-only auth.
# wiz-ignore: acceptance-test fixture — ephemeral server, no data, Entra-only auth
resource "azurerm_mssql_firewall_rule" "open" {
  name             = "allow-all"
  server_id        = azurerm_mssql_server.test.id
  start_ip_address = "0.0.0.0"
  end_ip_address   = "255.255.255.255"
}

resource "azurerm_mssql_database" "test" {
  name      = "${var.name_prefix}_db"
  server_id = azurerm_mssql_server.test.id
  sku_name  = "Basic"

  short_term_retention_policy {
    retention_days = 1
  }

  tags = local.common_tags
}

resource "azurerm_postgresql_flexible_server" "test" {
  name                = "${var.name_prefix}-pg-${var.run_id}"
  resource_group_name = data.azurerm_resource_group.test.name
  location            = data.azurerm_resource_group.test.location

  version    = "16"
  sku_name   = "B_Standard_B1ms"
  storage_mb = 32768

  authentication {
    active_directory_auth_enabled = true
    password_auth_enabled         = false
    tenant_id                     = data.azuread_client_config.current.tenant_id
  }

  # Azure auto-assigns the zone; ignoring drift avoids forced standby swaps on re-apply.
  lifecycle {
    ignore_changes = [zone]
  }

  tags = local.common_tags
}

resource "azurerm_postgresql_flexible_server_active_directory_administrator" "test" {
  server_name         = azurerm_postgresql_flexible_server.test.name
  resource_group_name = data.azurerm_resource_group.test.name
  tenant_id           = data.azuread_client_config.current.tenant_id
  object_id           = var.entra_admin_object_id
  principal_name      = local.admin_principal_name
  principal_type      = data.azuread_directory_object.admin.type

  # Azure-side LRO frequently exceeds the 30m default on both create and delete.
  timeouts {
    create = "60m"
    delete = "60m"
  }
}

# Test-only: open firewall. Server has no data and Entra-only auth.
# wiz-ignore: acceptance-test fixture — ephemeral server, no data, Entra-only auth
resource "azurerm_postgresql_flexible_server_firewall_rule" "open" {
  name             = "allow-all"
  server_id        = azurerm_postgresql_flexible_server.test.id
  start_ip_address = "0.0.0.0"
  end_ip_address   = "255.255.255.255"

  timeouts {
    create = "60m"
    delete = "60m"
  }
}

resource "azurerm_postgresql_flexible_server_database" "test" {
  name      = "${var.name_prefix}_pg_db"
  server_id = azurerm_postgresql_flexible_server.test.id
  charset   = "UTF8"
  collation = "en_US.utf8"
}
