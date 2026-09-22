resource "aws_secretsmanager_secret" "references" {
  for_each = toset(var.secret_names)

  name                    = "${var.name_prefix}${each.value}"
  recovery_window_in_days = 7
  tags                    = var.tags
}
