variable "subscription_id" {
  description = "Subscription hosting the test RG and servers. Set explicitly because azurerm v4 no longer falls back to the CLI's default sub."
  type        = string
}

variable "resource_group_name" {
  description = "Existing RG that will hold the test servers."
  type        = string
}

variable "run_id" {
  description = "Suffix appended to every resource name. Pick anything unique-enough; a maintainer typically uses 'acc'."
  type        = string
}

variable "name_prefix" {
  description = "Resource-name prefix, combined with run_id for globally-unique Azure resource names."
  type        = string
  default     = "tfacc"
}

variable "mssql_server_identity_id" {
  description = "Resource ID of a user-assigned MI with Entra Directory Readers granted. Attached to the MSSQL server for Graph principal-resolution. Can be the CI identity itself if it's a UAMI with that role."
  type        = string
}

variable "entra_admin_object_id" {
  description = "Entra object ID of the single Entra admin for both servers. Pass your own user object_id for local apply (so you can connect from `az`); pass the CI managed identity / service principal object_id when the fixture is applied from CI. Whoever applies must also be that principal."
  type        = string
}
