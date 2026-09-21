output "run_mode" {
  description = "Evidence that this configuration remains plan-only."
  value       = "plan-only"
}

output "name_prefix" {
  description = "Required prefix for future customer-controlled resources."
  value       = local.name_prefix
}

output "enabled_features" {
  description = "Safe-default feature flags; all must be false by default."
  value = {
    runtime        = var.enable_runtime
    scheduler      = var.enable_scheduler
    qemu           = var.enable_qemu
    cognito_google = var.enable_cognito_google
    qemu_instances = var.qemu_instance_count
  }
}

output "runtime_resources" {
  description = "Identifiers emitted only when the reviewed runtime feature is explicitly enabled."
  value = var.enable_runtime ? {
    artifact_bucket = module.artifact_store[0].bucket_name
    queue_url       = module.operation_queue[0].queue_url
    registry_url    = module.container_registry[0].repository_url
    api_role_arn    = module.runtime_identity[0].api_role_arn
    worker_role_arn = module.runtime_identity[0].worker_role_arn
  } : null
}

output "qemu_resources" {
  description = "Simulator identifiers emitted only when the isolated QEMU feature is enabled."
  value = var.enable_qemu ? {
    instance_ids      = module.qemu_lab[0].instance_ids
    security_group_id = module.qemu_lab[0].security_group_id
  } : null
}
