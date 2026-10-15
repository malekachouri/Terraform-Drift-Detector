package model

import "time"

// Resource is the canonical normalized representation of a piece of
// infrastructure. Both the Terraform state (expected) and the cloud API
// (actual) are converted into this shape so they can be compared 1:1.
type Resource struct {
	ID         string            `json:"id"` // provider/type/cloud_id — the join key
	Provider   string            `json:"provider"`
	Type       string            `json:"type"`
	CloudID    string            `json:"cloud_id"`
	Name       string            `json:"name"`
	Attributes map[string]any    `json:"attributes"`
	Tags       map[string]string `json:"tags"`
	Region     string            `json:"region"`
	Source     string            `json:"source"` // "state" or "cloud"
	Module     string            `json:"module,omitempty"`
	Address    string            `json:"address,omitempty"` // terraform address, e.g. module.net.aws_vpc.main
}

// DriftKind categorizes a drift finding.
type DriftKind string

const (
	DriftMissingInCloud  DriftKind = "missing_in_cloud"
	DriftExtraInCloud    DriftKind = "extra_in_cloud"
	DriftAttributeChange DriftKind = "attribute_changed"
	DriftTagsChanged     DriftKind = "tags_changed"
)

// Severity levels for drift findings.
const (
	SeverityInfo     = "info"
	SeverityWarning  = "warning"
	SeverityCritical = "critical"
)

// DriftFinding represents a single detected drift item.
type DriftFinding struct {
	Kind         DriftKind `json:"kind"`
	ResourceID   string    `json:"resource_id"`
	ResourceType string    `json:"resource_type"`
	ResourceName string    `json:"resource_name,omitempty"`
	Address      string    `json:"address,omitempty"`
	Field        string    `json:"field,omitempty"`
	Expected     any       `json:"expected,omitempty"`
	Actual       any       `json:"actual,omitempty"`
	Severity     string    `json:"severity"`
}

// DriftSummary aggregates finding counts.
type DriftSummary struct {
	TotalResources   int `json:"total_resources"`
	MissingInCloud   int `json:"missing_in_cloud"`
	ExtraInCloud     int `json:"extra_in_cloud"`
	AttributeChanges int `json:"attribute_changes"`
	TagChanges       int `json:"tag_changes"`
	TotalFindings    int `json:"total_findings"`
}

// DriftReport is the output of a drift scan.
type DriftReport struct {
	ScanID      string         `json:"scan_id"`
	WorkspaceID string         `json:"workspace_id,omitempty"`
	Workspace   string         `json:"workspace,omitempty"`
	StartedAt   time.Time      `json:"started_at"`
	CompletedAt time.Time      `json:"completed_at"`
	Status      string         `json:"status"`
	Summary     DriftSummary   `json:"summary"`
	Findings    []DriftFinding `json:"findings"`
	Errors      []string       `json:"errors,omitempty"`
}

// ScanStatus values.
const (
	ScanStatusRunning   = "running"
	ScanStatusCompleted = "completed"
	ScanStatusFailed    = "failed"
)

// Workspace represents a configured drift detection target.
type Workspace struct {
	ID       string          `json:"id" yaml:"-"`
	Name     string          `json:"name" yaml:"name"`
	Provider string          `json:"provider" yaml:"provider"`
	Endpoint string          `json:"endpoint,omitempty" yaml:"endpoint,omitempty"` // custom AWS endpoint, e.g. LocalStack
	State    StateConfig     `json:"state" yaml:"state"`
	Regions  []string        `json:"regions" yaml:"regions"`
	Compare  CompareConfig   `json:"compare" yaml:"compare"`
	Schedule *ScheduleConfig `json:"schedule,omitempty" yaml:"schedule,omitempty"`
}

// StateConfig describes where Terraform state is stored.
type StateConfig struct {
	Backend  string            `json:"backend" yaml:"backend"` // local | http | s3
	Path     string            `json:"path,omitempty" yaml:"path,omitempty"`
	Bucket   string            `json:"bucket,omitempty" yaml:"bucket,omitempty"`
	Key      string            `json:"key,omitempty" yaml:"key,omitempty"`
	Region   string            `json:"region,omitempty" yaml:"region,omitempty"`
	Endpoint string            `json:"endpoint,omitempty" yaml:"endpoint,omitempty"` // defaults to the workspace endpoint
	Extra    map[string]string `json:"extra,omitempty" yaml:"extra,omitempty"`
}

// CompareConfig controls drift comparison behavior.
type CompareConfig struct {
	IgnoreTags       []string `json:"ignore_tags" yaml:"ignore_tags"`
	IgnoreAttributes []string `json:"ignore_attributes" yaml:"ignore_attributes"`
	// DetectUnmanaged reports cloud resources that exist but are not in the
	// Terraform state ("extra_in_cloud"), e.g. buckets created by hand.
	DetectUnmanaged bool `json:"detect_unmanaged" yaml:"detect_unmanaged"`
	// IgnoreResources lists cloud IDs (bucket names, vpc-..., i-...) that are
	// never reported as unmanaged.
	IgnoreResources []string `json:"ignore_resources" yaml:"ignore_resources"`
}

// ScheduleConfig defines a cron schedule for a workspace.
type ScheduleConfig struct {
	Cron string `json:"cron" yaml:"cron"`
}
