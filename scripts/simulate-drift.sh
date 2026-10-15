#!/usr/bin/env bash
# Simulates "someone clicked in the AWS console": changes LocalStack resources
# behind Terraform's back so driftctl has something to detect.
#
# Usage: scripts/simulate-drift.sh [all|tags|sg|instance|vpc|versioning|delete|unmanaged|reset]
#
# By default commands run via `awslocal` inside the LocalStack container, so
# nothing but Docker is needed. Override with e.g.
#   AWSCLI="aws --endpoint-url http://localhost:4566" scripts/simulate-drift.sh
set -euo pipefail

AWSCLI=${AWSCLI:-"docker compose exec -T localstack awslocal"}
aws_() { $AWSCLI "$@"; }

say() { printf '\033[1;33m[drift]\033[0m %s\n' "$*"; }

lookup() { # lookup <describe-cmd> <query> <filters...>
  local cmd=$1 query=$2; shift 2
  aws_ ec2 "$cmd" --filters "$@" --query "$query" --output text
}

INSTANCE_ID=$(lookup describe-instances 'Reservations[0].Instances[0].InstanceId' \
  Name=tag:Name,Values=drift-demo-web Name=instance-state-name,Values=running,stopped)
SG_ID=$(lookup describe-security-groups 'SecurityGroups[0].GroupId' Name=group-name,Values=drift-demo-web)
VPC_ID=$(lookup describe-vpcs 'Vpcs[0].VpcId' Name=tag:Name,Values=drift-demo-vpc)

require() { [[ -n "$2" && "$2" != "None" ]] || { echo "$1 not found - did you run 'make tf-apply'?" >&2; exit 1; }; }

drift_tags() {
  require instance "$INSTANCE_ID"
  say "instance $INSTANCE_ID: env=dev -> env=prod, add owner=bob (tag drift)"
  aws_ ec2 create-tags --resources "$INSTANCE_ID" --tags Key=env,Value=prod Key=owner,Value=bob
}

drift_sg() {
  require "security group" "$SG_ID"
  say "security group $SG_ID: open SSH (22) to 0.0.0.0/0 (critical attribute drift)"
  aws_ ec2 authorize-security-group-ingress --group-id "$SG_ID" \
    --protocol tcp --port 22 --cidr 0.0.0.0/0 >/dev/null
}

drift_instance() {
  require instance "$INSTANCE_ID"
  say "instance $INSTANCE_ID: resize t3.micro -> t3.large (attribute drift)"
  aws_ ec2 stop-instances --instance-ids "$INSTANCE_ID" >/dev/null
  aws_ ec2 modify-instance-attribute --instance-id "$INSTANCE_ID" --instance-type Value=t3.large
  aws_ ec2 start-instances --instance-ids "$INSTANCE_ID" >/dev/null
}

drift_vpc() {
  require vpc "$VPC_ID"
  say "vpc $VPC_ID: disable DNS hostnames (attribute drift)"
  aws_ ec2 modify-vpc-attribute --vpc-id "$VPC_ID" --no-enable-dns-hostnames
}

drift_versioning() {
  say "bucket drift-demo-logs: enable versioning (attribute drift)"
  aws_ s3api put-bucket-versioning --bucket drift-demo-logs --versioning-configuration Status=Enabled
}

drift_delete() {
  say "bucket drift-demo-artifacts: delete it (missing_in_cloud)"
  aws_ s3 rb s3://drift-demo-artifacts --force >/dev/null
}

drift_unmanaged() {
  say "create bucket shadow-it-uploads by hand (extra_in_cloud / unmanaged)"
  aws_ s3api create-bucket --bucket shadow-it-uploads >/dev/null
}

reset() {
  say "removing hand-made bucket; run 'make tf-apply' to put everything else back"
  aws_ s3 rb s3://shadow-it-uploads --force >/dev/null 2>&1 || true
}

case "${1:-all}" in
  all)
    drift_tags; drift_sg; drift_instance; drift_vpc; drift_versioning; drift_delete; drift_unmanaged
    say "done - now run 'make scan'"
    ;;
  tags) drift_tags ;;
  sg) drift_sg ;;
  instance) drift_instance ;;
  vpc) drift_vpc ;;
  versioning) drift_versioning ;;
  delete) drift_delete ;;
  unmanaged) drift_unmanaged ;;
  reset) reset ;;
  *) echo "usage: $0 [all|tags|sg|instance|vpc|versioning|delete|unmanaged|reset]" >&2; exit 2 ;;
esac
