variable "name_prefix" {
  type = string
}

variable "artifact_bucket_arn" {
  type = string
}

variable "queue_arn" {
  type = string
}

variable "tags" {
  type = map(string)
}
