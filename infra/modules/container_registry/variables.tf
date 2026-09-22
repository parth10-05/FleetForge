variable "name" { type = string }
variable "tags" { type = map(string) }
variable "maximum_images" {
  type    = number
  default = 10

  validation {
    condition     = var.maximum_images >= 1 && var.maximum_images <= 50
    error_message = "maximum_images must be between one and fifty."
  }
}
