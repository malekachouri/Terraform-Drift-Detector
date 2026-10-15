# AWS provider pointed at LocalStack instead of real AWS.
# No AWS account is needed: credentials are the LocalStack dummies "test"/"test".
provider "aws" {
  region     = var.region
  access_key = "test"
  secret_key = "test"

  skip_credentials_validation = true
  skip_metadata_api_check     = true
  skip_requesting_account_id  = true
  s3_use_path_style           = true

  endpoints {
    ec2 = var.localstack_endpoint
    s3  = var.localstack_endpoint
    sts = var.localstack_endpoint
    iam = var.localstack_endpoint
  }

  default_tags {
    tags = {
      project    = "drift-demo"
      managed_by = "terraform"
    }
  }
}
