output "api_role_arn" {
  value = aws_iam_role.api.arn
}

output "worker_role_arn" {
  value = aws_iam_role.worker.arn
}

output "migration_role_arn" {
  value = aws_iam_role.migration.arn
}
