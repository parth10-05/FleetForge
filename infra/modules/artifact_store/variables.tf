variable "bucket_name" { type = string }
variable "tags" { type = map(string) }
variable "abort_incomplete_upload_days" {
  type    = number
  default = 1

  validation {
    condition     = var.abort_incomplete_upload_days >= 1 && var.abort_incomplete_upload_days <= 7
    error_message = "Incomplete uploads must be removed within seven days."
  }
}
