# Terraform Drift Detector (LocalStack edition)

Detect **configuration drift**, meaning the gap between what Terraform *thinks*
exists and what *actually* exists, without an AWS account. Everything runs
locally on [LocalStack](https://github.com/localstack/localstack) inside
Docker.

This is a re-creation of
[iam-veeramalla/terraform-drift-detector](https://github.com/iam-veeramalla/terraform-drift-detector)
(Apache-2.0) adapted to LocalStack. It also fixes several false positives
in the original (see [What changed vs. the original](#what-changed-vs-the-original)).

---

## 1. The concept: what is drift?

Terraform keeps a **state file** (`terraform.tfstate`). It records every
resource Terraform created and the attributes it had at the last
`apply`/`refresh`.

**Drift** is anything that changes the real infrastructure *outside*
Terraform, for example:

| Someone…                                  | Drift kind           |
|-------------------------------------------|----------------------|
| deletes a bucket in the console           | `missing_in_cloud`   |
| creates a bucket by hand ("shadow IT")    | `extra_in_cloud`     |
| resizes an instance / opens port 22       | `attribute_changed`  |
| retags a resource                         | `tags_changed`       |

`terraform plan` can show some of this, but it needs the full Terraform code
and provider credentials, and it can lock state. It also **cannot see
resources Terraform never created**. A drift detector only needs **read
access**: it reads the state file and asks the cloud API what really exists,
then diffs the two.

```
 expected  =  what the .tfstate says
 actual    =  what the cloud API returns right now
 drift     =  diff(expected, actual)
```

## 2. Architecture

```
                 ┌──────────────────────────── driftctl / drift-server ───────────────────────────┐
                 │                                                                                 │
 S3 / file /  ─► │ state.Reader ─► state.Extractor ─► []Resource (expected) ─┐                     │
 HTTP .tfstate   │  (raw bytes)     (parse + normalize)                       │                     │
                 │                                                            ├─► drift.Engine ─► DriftReport ─► SQLite
 Cloud APIs   ─► │ providers/aws ─────────────────────► []Resource (actual) ──┘     (join on ID,      │   │
 (EC2, S3)       │  (list + normalize)                                              diff attrs/tags) │   ├─► CLI table / JSON (exit 0/1/2)
                 │                                                                                 │   ├─► REST API
                 │ scheduler (cron) ──triggers──► scan.Scanner (orchestrates the pipeline)         │   └─► Web dashboard
                 └─────────────────────────────────────────────────────────────────────────────────┘
```

**The key idea is one normalized model.** Both sides are converted into the
same `model.Resource`:

```go
Resource{ ID: "aws/aws_instance/i-0abc", Type: "aws_instance", CloudID: "i-0abc",
          Attributes: {"instance_type": "t3.micro", ...}, Tags: {...} }
```

The `ID` (`provider/type/cloud-id`) is the join key. Once both sides look
alike, the engine's job is simple:

* ID only in expected → **missing_in_cloud** (critical)
* ID only in actual → **extra_in_cloud** (warning, opt-in via `detect_unmanaged`)
* ID in both → compare each attribute → **attribute_changed**; compare tags → **tags_changed**

The hard part is **normalization**: Terraform and the AWS API describe the
same thing differently. The code in `internal/canonical` and
`state.NormalizeAttributes` brings them to one form:

| Thing                | Terraform state                                         | AWS API                         | Canonical form                 |
|----------------------|---------------------------------------------------------|---------------------------------|--------------------------------|
| SG rule              | `{protocol:"-1", from_port:0, cidr_blocks:[…], self:…}` | `IpPermission{IpProtocol:"-1"}` | `"all all 0.0.0.0/0"`          |
| S3 versioning        | `versioning: [{enabled:false}]`                         | `GetBucketVersioning.Status`    | `versioning_enabled: false`    |
| VPC DNS hostnames    | `enable_dns_hostnames: true`                            | `DescribeVpcAttribute`          | `true`                         |
| tags                 | `tags_all` (incl. provider `default_tags`)              | `Tags`                          | `map[string]string`            |
| region               | `availability_zone: us-east-1a`                         | client region                   | `us-east-1`                    |

Terraform-only settings such as `force_destroy` are never compared, because
no cloud API can report them.

### Where LocalStack fits in

LocalStack emulates the AWS APIs on a single port (`:4566`). The only change
the detector needs is **where it sends API calls**:

```go
// internal/awsclient
awsconfig.LoadDefaultConfig(ctx,
    awsconfig.WithRegion(region),
    awsconfig.WithBaseEndpoint("http://localhost:4566"),             // ← LocalStack
    awsconfig.WithCredentialsProvider(static("test", "test")))       // ← dummy creds
s3.NewFromConfig(cfg, func(o *s3.Options){ o.UsePathStyle = true }) // ← no bucket DNS
```

Terraform gets the same treatment in `terraform/provider.tf` (`endpoints {}`)
and `terraform/versions.tf` (S3 backend in LocalStack).

```
┌────────────────────────── docker compose ──────────────────────────┐
│                                                                    │
│  localstack:4566  ◄── terraform apply  (creates VPC, subnet, SG,   │
│   ├─ EC2 (mock)        EC2, 2 buckets; state → s3://tf-state)       │
│   ├─ S3   ◄────────── scripts/simulate-drift.sh (manual changes)   │
│   └─ s3://tf-state                                                 │
│        ▲  ▲                                                        │
│        │  └──────── drift-server:8080 (API + dashboard + cron)     │
│        └─────────── driftctl (CLI on your host)                    │
└────────────────────────────────────────────────────────────────────┘
```

### Code layout

| Path                         | Role |
|------------------------------|------|
| `cmd/driftctl`               | CLI: `scan`, `report`, `workspace list`, `schedule create` |
| `cmd/drift-server`           | REST API + dashboard + cron scheduler |
| `internal/state`             | Read state (local/HTTP/S3) and extract normalized resources |
| `internal/providers/aws`     | List live EC2/VPC/subnet/SG/S3 resources, normalized |
| `internal/canonical`         | Shared canonical forms (SG rules, sorted lists) |
| `internal/awsclient`         | AWS SDK clients for AWS **or** LocalStack |
| `internal/drift`             | The diff engine + summary |
| `internal/scan`              | Orchestrates one scan: state → cloud → diff → store |
| `internal/store`             | SQLite (pure Go, no CGO): workspaces, scans, schedules |
| `internal/scheduler`         | Cron-based periodic scans |
| `internal/api`, `web/`       | REST API and embedded dashboard |
| `terraform/`                 | Demo infrastructure targeting LocalStack |
| `localstack/init/ready.d/`   | Creates the `tf-state` bucket at LocalStack start-up |
| `scripts/simulate-drift.sh`  | Makes "console changes" so there is drift to find |

Supported resource types: `aws_vpc`, `aws_subnet`, `aws_security_group`,
`aws_instance`, `aws_s3_bucket`. Other types in your state are listed in the
report as "not checked" and are never reported as false "missing".

---

## 3. Run it

**Requirements:** Docker with Compose v2. Go 1.24+ is needed only to build
the CLI on your host. Terraform is optional because `make` runs it in a
container by default.

```bash
# 1. Start LocalStack + drift-server
make up                 # = docker compose up -d --build --wait

# 2. Create the infrastructure in LocalStack (Terraform in Docker)
make tf-apply           # have terraform installed? -> make tf-apply TF="terraform -chdir=terraform"

# 3. Scan: nothing changed yet → exit code 0
make scan

# 4. Change things behind Terraform's back
make drift

# 5. Scan again → 8 findings, exit code 1
make scan
```

Or run steps 1–5 in one go with `make demo`.

Expected output of step 5:

```
FINDINGS
SEVERITY  KIND               RESOURCE                         FIELD                 EXPECTED (state)         ACTUAL (cloud)
CRITICAL  missing_in_cloud   aws_s3_bucket.artifacts          -                     -                        -
CRITICAL  attribute_changed  aws_security_group.web           ingress               ["tcp 80-80 0.0.0.0/0"]  ["tcp 22-22 0.0.0.0/0","tcp 80-80 0.0.0.0/0"]
WARNING   attribute_changed  aws_instance.web                 instance_type         t3.micro                 t3.large
WARNING   attribute_changed  aws_s3_bucket.logs               versioning_enabled    false                    true
WARNING   extra_in_cloud     aws_s3_bucket.shadow-it-uploads  -                     -                        -
WARNING   attribute_changed  aws_vpc.main                     enable_dns_hostnames  true                     false
INFO      tags_changed       aws_instance.web                 tags.env              dev                      prod
INFO      tags_changed       aws_instance.web                 tags.owner            -                        bob
```

Then open **http://localhost:8080** and click **Run Scan** to see the same
report in the dashboard. drift-server also re-scans every 5 minutes (see
`schedule.cron` in `configs/driftctl.yaml`).

**Remediate** (make the cloud match the code again):

```bash
make tf-apply                        # Terraform reverts what it manages
./scripts/simulate-drift.sh reset    # remove the hand-made bucket
make scan                            # → No drift detected
```

> Compare with `make tf-plan` after `make drift`. Terraform reports the
> instance, SG, VPC and missing bucket, but it misses the hand-made
> `shadow-it-uploads` bucket because it is not in the state. It also misses
> the S3 versioning change, because the AWS provider v5 treats that
> attribute as computed. The drift detector catches both.

You can also apply individual kinds of drift:
`./scripts/simulate-drift.sh tags|sg|instance|vpc|versioning|delete|unmanaged`.

**No Go installed?** Use the CLI that ships in the server image:

```bash
docker compose exec drift-server driftctl scan --config /app/configs/driftctl.yaml --workspace localstack
```

## 4. CLI reference

```bash
# Workspace from config
driftctl scan --config configs/driftctl.yaml --workspace localstack

# Ad-hoc: state in LocalStack S3
driftctl scan --localstack --state-bucket tf-state --state-key demo/terraform.tfstate

# Ad-hoc: local state file, JSON output, ignore unmanaged resources
driftctl scan --localstack --state terraform.tfstate --detect-unmanaged=false --output json

# Saved reports / workspaces / schedules
driftctl report <scan-id> --config configs/driftctl.yaml
driftctl workspace list --config configs/driftctl.yaml
driftctl schedule create --config configs/driftctl.yaml --workspace localstack --cron "0 * * * *"
```

Exit codes: `0` = no drift, `1` = drift detected, `2` = error. This makes the
CLI usable as a CI gate.

`--endpoint URL` (or the standard `AWS_ENDPOINT_URL` env var) points the
scanner at any AWS-compatible endpoint. Leave it empty to target real AWS
with your normal credentials.

## 5. Configuration

`configs/driftctl.yaml`. `${VAR:-default}` is expanded from the environment,
so one file works from the host (`localhost:4566`) and from Compose
(`localstack:4566`):

```yaml
database: ${DRIFTCTL_DB:-driftctl.db}
api:
  addr: ":8080"
  # api_key: "change-me"           # protects /api/* via X-API-Key header
workspaces:
  - name: localstack
    provider: aws
    endpoint: ${LOCALSTACK_ENDPOINT:-http://localhost:4566}
    state: { backend: s3, bucket: tf-state, key: demo/terraform.tfstate, region: us-east-1 }
    regions: [us-east-1]
    compare:
      detect_unmanaged: true
      ignore_tags: []                 # e.g. [last_deployed]
      ignore_attributes: []           # e.g. [ami] or [aws_instance.ami]
      ignore_resources: []            # cloud IDs never reported as unmanaged
    schedule:
      cron: "*/5 * * * *"
```

The bucket holding the Terraform state is ignored automatically.

## 6. REST API

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Health check |
| GET | `/api/v1/workspaces` | List workspaces |
| POST | `/api/v1/workspaces` | Create workspace |
| GET/DELETE | `/api/v1/workspaces/{id}` | Get / delete workspace |
| POST | `/api/v1/workspaces/{id}/scans` | Trigger scan (returns the report) |
| GET | `/api/v1/workspaces/{id}/scans` | Scans of a workspace |
| GET | `/api/v1/scans` | Recent scans |
| GET | `/api/v1/scans/{id}/report?format=json\|table` | Drift report |
| PUT/DELETE | `/api/v1/workspaces/{id}/schedules` | Set / remove cron schedule |

```bash
make server-scan     # triggers a scan via the API
```

## 7. LocalStack notes

* The image is pinned to **`localstack/localstack:4.14.0`**, the last
  Community release that starts **without an account or auth token**.
  Newer releases (`2026.x`) need a free LocalStack account and
  `LOCALSTACK_AUTH_TOKEN`.
* EC2 in LocalStack Community is a *mock*: instances are API records, not
  VMs. That is all a drift detector needs.
* LocalStack state is in memory: `make down` (or a restart) wipes it. Run
  `make tf-apply` again afterwards. If Terraform complains about stale local
  state, delete `terraform/.terraform`.
* The default VPC, default subnets and `default` security groups are skipped
  automatically. AWS creates them, not Terraform.

## What changed vs. the original

* **LocalStack support**: endpoint override, dummy credentials, path-style
  S3, and env expansion in the config.
* **Unmanaged resources are actually detected.** The original only described
  IDs taken from the state, so `extra_in_cloud` could never fire. Resources
  are now *listed*, with pagination.
* **No false positives on a fresh apply.** The original compared values that
  were never fetched (`enable_dns_*` was always `nil`), Terraform-only
  fields (`force_destroy`), a list vs. a map (S3 `versioning`), and
  differently shaped SG rules. All of these reported drift on a
  perfectly-synced stack.
* **Deleted resources no longer break the scan.** Describing a deleted
  instance ID returned `InvalidInstanceID.NotFound`, which failed the whole
  call.
* Resource types the scanner can't check are no longer reported as
  "missing".
* Workspace IDs are stable across restarts, so schedules aren't orphaned.
* A cloud fetch that returns nothing fails the scan instead of reporting
  everything as missing.
* Pure-Go SQLite (no CGO), the dashboard is embedded in the binary, and
  findings are sorted by severity and show the Terraform address.

## License

Apache License 2.0. See [LICENSE](LICENSE). Based on
[iam-veeramalla/terraform-drift-detector](https://github.com/iam-veeramalla/terraform-drift-detector).
