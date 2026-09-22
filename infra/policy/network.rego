package fleetforge.network

public_cidrs := {"0.0.0.0/0", "::/0"}

deny contains msg if {
  change := input.resource_changes[_]
  change.type == "aws_security_group_rule"
  after := object.get(change.change, "after", {})
  object.get(after, "type", "") == "ingress"
  cidr := object.get(after, "cidr_blocks", [])[_]
  public_cidrs[cidr]
  msg := sprintf("%s exposes public IPv4 ingress", [change.address])
}

deny contains msg if {
  change := input.resource_changes[_]
  change.type == "aws_security_group_rule"
  after := object.get(change.change, "after", {})
  object.get(after, "type", "") == "ingress"
  cidr := object.get(after, "ipv6_cidr_blocks", [])[_]
  public_cidrs[cidr]
  msg := sprintf("%s exposes public IPv6 ingress", [change.address])
}
