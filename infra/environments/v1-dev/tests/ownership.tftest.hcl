run "run_one_is_plan_only" {
  command = plan

  assert {
    condition     = output.run_mode == "plan-only"
    error_message = "The plan-only foundation may not become apply-capable."
  }
}
