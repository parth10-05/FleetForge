variable "name_prefix" { type = string }
variable "availability_zone" { type = string }
variable "ami_id" { type = string }
variable "tags" { type = map(string) }
variable "vpc_cidr" {
  type    = string
  default = "10.42.0.0/16"
}

variable "subnet_cidr" {
  type    = string
  default = "10.42.1.0/24"
}

variable "instance_type" {
  type    = string
  default = "t3.medium"
}

variable "root_volume_gib" {
  type    = number
  default = 30
}

variable "instance_count" {
  type    = number
  default = 0
}
