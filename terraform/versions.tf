terraform {
  required_version = ">= 1.6.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.80"
    }
  }

  # Remote state stored in an S3 bucket *inside LocalStack*.
  # The bucket is created automatically by localstack/init/ready.d/01-state-bucket.sh.
  # driftctl reads this exact object to learn the "expected" state.
  backend "s3" {
    bucket = "tf-state"
    key    = "demo/terraform.tfstate"
    region = "us-east-1"

    endpoints = {
      s3 = "http://localhost:4566"
    }
    use_path_style              = true
    access_key                  = "test"
    secret_key                  = "test"
    skip_credentials_validation = true
    skip_requesting_account_id  = true
    skip_metadata_api_check     = true
    skip_region_validation      = true
  }
}
