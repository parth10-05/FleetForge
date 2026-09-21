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

# This foundation intentionally declares no customer-managed resources. Future
# modules are introduced only after their provider-specific review gates.
