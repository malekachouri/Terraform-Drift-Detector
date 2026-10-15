variable "region" {
  description = "AWS region emulated by LocalStack"
  type        = string
  default     = "us-east-1"
}

variable "localstack_endpoint" {
  description = "LocalStack edge endpoint"
  type        = string
  default     = "http://localhost:4566"
}

variable "env" {
  description = "Environment tag"
  type        = string
  default     = "dev"
}

variable "ami" {
  description = "AMI id. LocalStack ships a set of mock AMIs; this one is Ubuntu 14.04."
  type        = string
  default     = "ami-1e749f67"
}

variable "instance_type" {
  type    = string
  default = "t3.micro"
}
