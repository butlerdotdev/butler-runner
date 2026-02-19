# Copyright 2026 The Butler Authors.
# SPDX-License-Identifier: Apache-2.0

variable "project_id" {
  description = "GCP project ID"
  type        = string
}

variable "bucket_name" {
  description = "Name of the GCS bucket (must be globally unique)"
  type        = string
}

variable "location" {
  description = "GCS bucket location (e.g. US, EU, us-central1)"
  type        = string
  default     = "US"
}

variable "storage_class" {
  description = "Storage class: STANDARD, NEARLINE, COLDLINE, ARCHIVE"
  type        = string
  default     = "STANDARD"
}

variable "versioning_enabled" {
  description = "Enable object versioning"
  type        = bool
  default     = true
}

variable "lifecycle_age_days" {
  description = "Delete objects older than this many days (null to disable)"
  type        = number
  default     = null
}

variable "labels" {
  description = "Labels to apply to the bucket"
  type        = map(string)
  default     = {}
}
