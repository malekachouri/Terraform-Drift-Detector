#!/usr/bin/env bash
# End-to-end walkthrough. Needs Docker (+ Go to build the CLI, or use the
# driftctl binary inside the drift-server container: DRIFTCTL="docker compose exec -T drift-server driftctl").
set -euo pipefail
cd "$(dirname "$0")/.."

TF=${TF:-"docker compose run --rm terraform"}
DRIFTCTL=${DRIFTCTL:-./bin/driftctl}
CONFIG=${CONFIG:-configs/driftctl.yaml}

step() { printf '\n\033[1;36m==> %s\033[0m\n' "$*"; }
scan() { $DRIFTCTL scan --config "$CONFIG" --workspace localstack || true; }

step "1/6 Start LocalStack + drift-server"
docker compose up -d --build --wait

if [[ "$DRIFTCTL" == ./bin/* ]]; then
  step "Build driftctl"
  go build -o bin/driftctl ./cmd/driftctl
fi

step "2/6 terraform apply (into LocalStack, state stored in s3://tf-state)"
$TF init -input=false >/dev/null
$TF apply -auto-approve -input=false

step "3/6 Scan right after apply -> expect NO drift"
scan

step "4/6 Simulate manual changes behind Terraform's back"
./scripts/simulate-drift.sh all

step "5/6 Scan again -> expect drift"
scan

step "6/6 Dashboard"
echo "Open http://localhost:8080 and press 'Run Scan'."
echo "Remediate with:  make tf-apply && ./scripts/simulate-drift.sh reset"
