variable "name_prefix" { type = string }
variable "tags" { type = map(string) }

variable "secret_names" {
  type = set(string)
  default = [
    "database-application",
    "database-migration",
    "database-readonly",
    "oauth-google",
    "simulator-credential-reference",
  ]
}
