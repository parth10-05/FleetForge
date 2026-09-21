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

variable "account_suffix" {
  description = "Non-secret lowercase account discriminator included in globally unique bucket names."
  type        = string
  default     = "unconfigured"

  validation {
    condition     = can(regex("^[a-z0-9][a-z0-9-]{1,30}$", var.account_suffix))
    error_message = "account_suffix must be a lowercase identifier containing letters, digits, and hyphens."
  }
}

variable "abort_incomplete_upload_days" {
  description = "Bound for aborting partial uploads to the private artifact bucket."
  type        = number
  default     = 1
}

variable "queue_visibility_timeout_seconds" {
  description = "SQS operation visibility timeout; tune only after measured worker execution evidence."
  type        = number
  default     = 120
}

variable "queue_max_receive_count" {
  description = "Maximum receives before an operation moves to the dead-letter queue."
  type        = number
  default     = 5
}

variable "registry_maximum_images" {
  description = "Maximum immutable image revisions retained by the registry lifecycle policy."
  type        = number
  default     = 10
}

variable "api_image" {
  description = "Immutable API image reference. Set to an approved digest only during a runtime enablement."
  type        = string
  default     = "registry.invalid/fleetforge/api@sha256:pending"
}

variable "worker_image" {
  description = "Immutable worker image reference. Set to an approved digest only during a runtime enablement."
  type        = string
  default     = "registry.invalid/fleetforge/worker@sha256:pending"
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

variable "cognito_callback_urls" {
  description = "Approved HTTPS application callback URLs. Replace placeholders only during the credentialed configuration run."
  type        = list(string)
  default     = ["https://example.invalid/auth/callback"]
}

variable "cognito_logout_urls" {
  description = "Approved HTTPS application logout URLs. Replace placeholders only during the credentialed configuration run."
  type        = list(string)
  default     = ["https://example.invalid/logout"]
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

variable "qemu_availability_zone" {
  description = "Availability Zone for the isolated simulator subnet; set only during a reviewed QEMU enablement."
  type        = string
  default     = "us-east-1a"
}

variable "qemu_ami_id" {
  description = "Reviewed AMI ID for the synthetic QEMU host. This must never be a production image."
  type        = string
  default     = "ami-00000000000000000"

  validation {
    condition     = can(regex("^ami-[0-9a-f]{17}$", var.qemu_ami_id))
    error_message = "qemu_ami_id must be an AMI identifier."
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
