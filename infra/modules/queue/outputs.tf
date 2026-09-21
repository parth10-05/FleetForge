output "queue_arn" { value = aws_sqs_queue.work.arn }
output "queue_url" { value = aws_sqs_queue.work.url }
output "dead_letter_queue_arn" { value = aws_sqs_queue.dead_letter.arn }
