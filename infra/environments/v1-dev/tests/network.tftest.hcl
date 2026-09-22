mock_provider "aws" {}

run "run_one_has_plan_only_output" {
  command = plan

  assert {
    condition     = output.run_mode == "plan-only"
    error_message = "The plan-only foundation must not plan networking or any AWS resource."
  }
}
