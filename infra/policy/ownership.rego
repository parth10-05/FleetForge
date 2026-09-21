package fleetforge.ownership

required_tags := {
  "Project": "FleetForge",
  "Environment": "v1-dev",
  "ManagedBy": "terraform",
  "CostScope": "ps-fleetforge",
  "DataClass": "synthetic-only",
  "VersionScope": "v1",
}

deny contains msg if {
  change := input.resource_changes[_]
  after := object.get(change.change, "after", {})
  name := object.get(after, "name", "")
  name != ""
  not startswith(name, "ps-fleetforge-v1-dev-")
  msg := sprintf("%s uses an unowned name %q", [change.address, name])
}

deny contains msg if {
  change := input.resource_changes[_]
  after := object.get(change.change, "after", {})
  tags := object.get(after, "tags", {})
  expected := required_tags[key]
  tags[key] != expected
  msg := sprintf("%s is missing required tag %s=%s", [change.address, key, expected])
}

deny contains msg if {
  change := input.resource_changes[_]
  after := object.get(change.change, "after", {})
  tags := object.get(after, "tags", {})
  not tags.Owner
  msg := sprintf("%s is missing an Owner tag", [change.address])
}

deny contains msg if {
  change := input.resource_changes[_]
  after := object.get(change.change, "after", {})
  tags := object.get(after, "tags", {})
  not tags.ExpiresAt
  msg := sprintf("%s is missing an ExpiresAt tag", [change.address])
}
