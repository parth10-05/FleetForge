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

module "runtime_identity" {
  count  = var.enable_runtime ? 1 : 0
  source = "../../modules/runtime_identity"

  name_prefix         = local.name_prefix
  artifact_bucket_arn = module.artifact_store[0].bucket_arn
  queue_arn           = module.operation_queue[0].queue_arn
  tags                = local.required_tags
}

module "runtime_shells" {
  count  = var.enable_runtime ? 1 : 0
  source = "../../modules/runtime_shells"

  name_prefix          = local.name_prefix
  region               = var.region
  api_image            = var.api_image
  worker_image         = var.worker_image
  api_task_role_arn    = module.runtime_identity[0].api_role_arn
  worker_task_role_arn = module.runtime_identity[0].worker_role_arn
  tags                 = local.required_tags
}

module "qemu_lab" {
  count  = var.enable_qemu ? 1 : 0
  source = "../../modules/qemu_lab"

  name_prefix       = local.name_prefix
  availability_zone = var.qemu_availability_zone
  ami_id            = var.qemu_ami_id
  instance_count    = var.qemu_instance_count
  tags              = local.required_tags
}

module "cognito" {
  count  = var.enable_cognito_google ? 1 : 0
  source = "../../modules/cognito"

  name_prefix   = local.name_prefix
  callback_urls = var.cognito_callback_urls
  logout_urls   = var.cognito_logout_urls
  tags          = local.required_tags
}
