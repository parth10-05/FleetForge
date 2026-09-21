package fleetforge.free_plan

forbidden_types := {
  "aws_nat_gateway",
  "aws_lb",
  "aws_alb",
  "aws_eip",
  "aws_db_instance",
  "aws_rds_cluster",
  "aws_eks_cluster",
  "aws_marketplace_agreement",
  "aws_savingsplans_savings_plan",
  "aws_supportplan",
}

deny contains msg if {
  change := input.resource_changes[_]
  forbidden_types[change.type]
  msg := sprintf("%s is forbidden in this development environment", [change.address])
}

deny contains msg if {
  change := input.resource_changes[_]
  change.type == "aws_autoscaling_group"
  after := object.get(change.change, "after", {})
  object.get(after, "max_size", 0) > 1
  msg := sprintf("%s has unbounded autoscaling", [change.address])
}
