resource "aws_ecs_cluster" "main" {
  name = "${var.name_prefix}runtime"
  setting {
    name  = "containerInsights"
    value = "disabled"
  }
  tags = var.tags
}

data "aws_iam_policy_document" "task_assume_role" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "execution" {
  name               = "${var.name_prefix}task-execution"
  assume_role_policy = data.aws_iam_policy_document.task_assume_role.json
  tags               = var.tags
}

resource "aws_iam_role_policy_attachment" "execution" {
  role       = aws_iam_role.execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

resource "aws_cloudwatch_log_group" "runtime" {
  name              = "/fleetforge/${var.name_prefix}runtime"
  retention_in_days = 14
  tags              = var.tags
}

resource "aws_ecs_task_definition" "api" {
  family                   = "${var.name_prefix}api"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = "256"
  memory                   = "512"
  execution_role_arn       = aws_iam_role.execution.arn
  task_role_arn            = var.api_task_role_arn
  container_definitions    = jsonencode([{ name = "api", image = var.api_image, essential = true, logConfiguration = { logDriver = "awslogs", options = { awslogs-group = aws_cloudwatch_log_group.runtime.name, awslogs-region = var.region, awslogs-stream-prefix = "api" } } }])
  tags                     = var.tags
}

resource "aws_ecs_task_definition" "worker" {
  family                   = "${var.name_prefix}worker"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = "256"
  memory                   = "512"
  execution_role_arn       = aws_iam_role.execution.arn
  task_role_arn            = var.worker_task_role_arn
  container_definitions    = jsonencode([{ name = "worker", image = var.worker_image, essential = true, logConfiguration = { logDriver = "awslogs", options = { awslogs-group = aws_cloudwatch_log_group.runtime.name, awslogs-region = var.region, awslogs-stream-prefix = "worker" } } }])
  tags                     = var.tags
}
