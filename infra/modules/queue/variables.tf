variable "name" { type = string }
variable "tags" { type = map(string) }
variable "visibility_timeout_seconds" {
  type    = number
  default = 120

  validation {
    condition     = var.visibility_timeout_seconds >= 30 && var.visibility_timeout_seconds <= 43200
    error_message = "Visibility timeout must be bounded."
  }
}
variable "message_retention_seconds" {
  type    = number
  default = 345600
}

variable "dead_letter_retention_seconds" {
  type    = number
  default = 1209600
}
variable "max_receive_count" {
  type    = number
  default = 5

  validation {
    condition     = var.max_receive_count >= 1 && var.max_receive_count <= 10
    error_message = "max_receive_count must be between one and ten."
  }
}
