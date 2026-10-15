output "vpc_id" {
  value = aws_vpc.main.id
}

output "subnet_id" {
  value = aws_subnet.public.id
}

output "security_group_id" {
  value = aws_security_group.web.id
}

output "instance_id" {
  value = aws_instance.web.id
}

output "bucket" {
  value = aws_s3_bucket.logs.bucket
}

output "artifacts_bucket" {
  value = aws_s3_bucket.artifacts.bucket
}
