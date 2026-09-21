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
