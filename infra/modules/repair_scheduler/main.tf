data "aws_iam_policy_document" "scheduler_assume_role" {
  statement {
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["scheduler.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "scheduler" {
  name               = "${var.name_prefix}repair-scheduler"
  assume_role_policy = data.aws_iam_policy_document.scheduler_assume_role.json
  tags               = var.tags
}

resource "aws_iam_role_policy" "run_task" {
  name = "bounded-repair-task"
  role = aws_iam_role.scheduler.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["ecs:RunTask"]
      Resource = var.task_definition_arn
      }, {
      Effect   = "Allow"
      Action   = ["iam:PassRole"]
      Resource = var.task_role_arns
      Condition = {
        StringEquals = {
          "iam:PassedToService" = "ecs-tasks.amazonaws.com"
        }
      }
    }]
  })
}

resource "aws_scheduler_schedule" "repair" {
  name                         = "${var.name_prefix}repair"
  schedule_expression          = var.schedule_expression
  state                        = var.enabled ? "ENABLED" : "DISABLED"
  schedule_expression_timezone = "UTC"

  flexible_time_window {
    mode = "OFF"
  }

  target {
    arn      = var.ecs_cluster_arn
    role_arn = aws_iam_role.scheduler.arn
    input    = jsonencode({ purpose = "bounded-overdue-operation-repair" })

    ecs_parameters {
      task_definition_arn = var.task_definition_arn
      launch_type         = "FARGATE"

      network_configuration {
        subnets         = var.subnet_ids
        security_groups = var.security_group_ids
      }
    }
  }
}
