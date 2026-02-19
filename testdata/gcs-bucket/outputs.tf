# Copyright 2026 The Butler Authors.
# SPDX-License-Identifier: Apache-2.0

output "bucket_name" {
  description = "The name of the bucket"
  value       = google_storage_bucket.this.name
}

output "bucket_url" {
  description = "The base URL of the bucket (gs://...)"
  value       = google_storage_bucket.this.url
}

output "bucket_self_link" {
  description = "The URI of the created resource"
  value       = google_storage_bucket.this.self_link
}
