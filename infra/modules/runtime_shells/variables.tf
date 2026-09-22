variable "name_prefix" { type = string }
variable "region" { type = string }
variable "api_image" { type = string }
variable "worker_image" { type = string }
variable "api_task_role_arn" { type = string }
variable "worker_task_role_arn" { type = string }
variable "tags" { type = map(string) }
