locals {
  name_prefix = "ps-fleetforge-${var.environment}-"

  feature_enabled = anytrue([
    var.enable_runtime,
    var.enable_scheduler,
    var.enable_qemu,
    var.enable_cognito_google,
  ])

  required_tags = {
    Project      = "FleetForge"
    Owner        = var.owner
    Environment  = var.environment
    ManagedBy    = "terraform"
    CostScope    = "ps-fleetforge"
    DataClass    = "synthetic-only"
    VersionScope = "v1"
    ExpiresAt    = coalesce(var.apply_expires_at, "not-provisioned")
  }
}

module "artifact_store" {
  count  = var.enable_runtime ? 1 : 0
  source = "../../modules/artifact_store"

  bucket_name                  = "${local.name_prefix}artifacts-${var.account_suffix}"
  abort_incomplete_upload_days = var.abort_incomplete_upload_days
  tags                         = local.required_tags
}

module "operation_queue" {
  count  = var.enable_runtime ? 1 : 0
  source = "../../modules/queue"

  name                       = "${local.name_prefix}operations"
  visibility_timeout_seconds = var.queue_visibility_timeout_seconds
  max_receive_count          = var.queue_max_receive_count
  tags                       = local.required_tags
}

module "container_registry" {
  count  = var.enable_runtime ? 1 : 0
  source = "../../modules/container_registry"

  name           = "${local.name_prefix}service"
  maximum_images = var.registry_maximum_images
  tags           = local.required_tags
}
