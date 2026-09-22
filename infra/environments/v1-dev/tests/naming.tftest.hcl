mock_provider "aws" {}

run "default_naming_is_owned" {
  command = plan

  assert {
    condition     = output.name_prefix == "ps-fleetforge-v1-dev-"
    error_message = "The v1-dev naming prefix must remain stable."
  }
}
