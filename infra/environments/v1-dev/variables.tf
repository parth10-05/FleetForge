variable "region" {
  description = "Single AWS region for v1-dev. Cross-region resources are forbidden."
  type        = string
  default     = "us-east-1"

  validation {
    condition     = var.region == "us-east-1"
    error_message = "This configuration permits only us-east-1. A region change requires a dated review before apply-capable infrastructure exists."
  }
}

variable "owner" {
  description = "Short ownership identifier used in required tags."
  type        = string
  default     = "ps"

  validation {
    condition     = can(regex("^[a-z0-9][a-z0-9-]{0,30}$", var.owner))
    error_message = "owner must be a non-empty lowercase identifier containing only letters, digits, and hyphens."
  }
}

variable "environment" {
  description = "Environment tag and mandatory resource-name component."
  type        = string
  default     = "v1-dev"

  validation {
    condition     = var.environment == "v1-dev"
    error_message = "This configuration permits only the v1-dev environment."
  }
}

variable "apply_expires_at" {
  description = "UTC RFC3339 expiry required before any future feature can be enabled."
  type        = string
  default     = null
  nullable    = true

  validation {
    condition     = var.apply_expires_at == null || can(formatdate("YYYY-MM-DD'T'hh:mm:ss'Z'", var.apply_expires_at))
    error_message = "apply_expires_at must be an RFC3339 UTC timestamp when supplied."
  }
}

variable "enable_runtime" {
  type        = bool
  default     = false
  description = "Reserved for a later reviewed ECS/runtime module."
}

variable "enable_scheduler" {
  type        = bool
  default     = false
  description = "Reserved for a later reviewed EventBridge Scheduler module."
}

variable "enable_qemu" {
  type        = bool
  default     = false
  description = "Reserved for the isolated QEMU capability-spike module."
}

variable "enable_cognito_google" {
  type        = bool
  default     = false
  description = "Reserved for a later Cognito/Google federation module."
}

variable "qemu_instance_count" {
  type        = number
  default     = 0
  description = "Future QEMU EC2 maximum; the default remains zero."

  validation {
    condition     = contains([0, 1], var.qemu_instance_count)
    error_message = "qemu_instance_count must be zero or one."
  }
}

check "feature_enablement_requires_bounded_expiry" {
  assert {
    condition = !local.feature_enabled || (
      var.apply_expires_at != null &&
      timecmp(var.apply_expires_at, timestamp()) == 1 &&
      timecmp(var.apply_expires_at, timeadd(timestamp(), "168h")) != 1
    )
    error_message = "Any enabled feature requires a future apply_expires_at no more than seven days away."
  }
}

check "qemu_count_requires_feature" {
  assert {
    condition     = var.qemu_instance_count == 0 || var.enable_qemu
    error_message = "A nonzero qemu_instance_count requires enable_qemu=true."
  }
}
