#!/bin/bash
# Runs inside the LocalStack container once it is ready.
# Creates the S3 bucket that holds the Terraform remote state.
set -euo pipefail

awslocal s3api create-bucket --bucket tf-state >/dev/null 2>&1 || true
awslocal s3api put-bucket-versioning --bucket tf-state --versioning-configuration Status=Enabled
echo "[init] terraform state bucket 's3://tf-state' is ready"
