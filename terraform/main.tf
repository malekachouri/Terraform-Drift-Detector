# Demo infrastructure: one of each resource type driftctl understands.

resource "aws_vpc" "main" {
  cidr_block           = "10.0.0.0/16"
  enable_dns_hostnames = true
  enable_dns_support   = true

  tags = {
    Name = "drift-demo-vpc"
    env  = var.env
  }
}

resource "aws_subnet" "public" {
  vpc_id                  = aws_vpc.main.id
  cidr_block              = "10.0.1.0/24"
  availability_zone       = "${var.region}a"
  map_public_ip_on_launch = false

  tags = {
    Name = "drift-demo-public"
    env  = var.env
  }
}

resource "aws_security_group" "web" {
  name        = "drift-demo-web"
  description = "Allow HTTP in, everything out"
  vpc_id      = aws_vpc.main.id

  ingress {
    description = "HTTP"
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name = "drift-demo-web"
    env  = var.env
  }
}

resource "aws_instance" "web" {
  ami                    = var.ami
  instance_type          = var.instance_type
  subnet_id              = aws_subnet.public.id
  vpc_security_group_ids = [aws_security_group.web.id]
  monitoring             = false

  tags = {
    Name = "drift-demo-web"
    env  = var.env
  }
}

resource "aws_s3_bucket" "logs" {
  bucket        = "drift-demo-logs"
  force_destroy = true

  tags = {
    Name = "drift-demo-logs"
    env  = var.env
  }
}

# A second bucket: the drift simulation deletes it out-of-band so driftctl
# reports it as "missing_in_cloud".
resource "aws_s3_bucket" "artifacts" {
  bucket        = "drift-demo-artifacts"
  force_destroy = true

  tags = {
    Name = "drift-demo-artifacts"
    env  = var.env
  }
}
