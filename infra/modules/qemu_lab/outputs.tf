output "instance_ids" { value = aws_instance.qemu[*].id }
output "security_group_id" { value = aws_security_group.instance.id }
