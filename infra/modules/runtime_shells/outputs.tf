output "cluster_arn" { value = aws_ecs_cluster.main.arn }
output "api_task_definition_arn" { value = aws_ecs_task_definition.api.arn }
output "worker_task_definition_arn" { value = aws_ecs_task_definition.worker.arn }
