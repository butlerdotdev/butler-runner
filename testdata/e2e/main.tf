# Copyright 2026 The Butler Authors.
# SPDX-License-Identifier: Apache-2.0
#
# Minimal E2E test module - no providers, no resources, instant execution.
# Used by hack/e2e-test.sh to validate butler-runner managed mode against
# a live butler-portal deployment.

output "result" {
  value = "e2e-test-success"
}

output "timestamp" {
  value = timestamp()
}
