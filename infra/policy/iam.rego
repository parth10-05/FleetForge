package fleetforge.iam

forbidden_actions := {
  "freetier:UpgradeAccountPlan",
  "organizations:*",
  "controltower:*",
}

deny contains msg if {
  change := input.resource_changes[_]
  change.type == "aws_iam_policy"
  document := object.get(change.change.after, "policy", "")
  action := forbidden_actions[_]
  contains(document, action)
  msg := sprintf("%s grants forbidden action %s", [change.address, action])
}
