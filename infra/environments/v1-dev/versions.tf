terraform {
  # Terraform 1.16.3 is selected for this foundation. Upgrade through a reviewed PR
  # which refreshes .terraform.lock.hcl and the CI tool version together.
  required_version = "~> 1.16.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.10"
    }
  }
}
