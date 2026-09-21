run "default_features_are_disabled" {
  command = plan

  assert {
    condition = output.enabled_features == {
      runtime        = false
      scheduler      = false
      qemu           = false
      cognito_google = false
      qemu_instances = 0
    }
    error_message = "The default must not enable a runtime, scheduler, QEMU, Cognito, or EC2 instance."
  }
}
