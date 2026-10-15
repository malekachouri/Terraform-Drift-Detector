package scan

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/malekachouri/terraform-drift-detector/internal/drift"
	"github.com/malekachouri/terraform-drift-detector/internal/model"
	"github.com/malekachouri/terraform-drift-detector/internal/providers"
	"github.com/malekachouri/terraform-drift-detector/internal/state"
	"github.com/malekachouri/terraform-drift-detector/internal/store"
)

// Scanner orchestrates drift detection scans.
type Scanner struct {
	StateReader state.Reader
	Providers   *providers.Registry
	Store       store.Store
}

// Options configures a single scan run.
type Options struct {
	WorkspaceID   string
	WorkspaceName string
	Provider      string
	Endpoint      string
	State         model.StateConfig
	Regions       []string
	Compare       model.CompareConfig
	SkipCloud     bool // state-only run, for testing
}

// OptionsFromWorkspace builds scan options for a stored workspace.
func OptionsFromWorkspace(ws *model.Workspace) Options {
	return Options{
		WorkspaceID:   ws.ID,
		WorkspaceName: ws.Name,
		Provider:      ws.Provider,
		Endpoint:      ws.Endpoint,
		State:         ws.State,
		Regions:       ws.Regions,
		Compare:       ws.Compare,
	}
}

func NewScanner(st store.Store) *Scanner {
	return &Scanner{
		StateReader: state.NewReader(),
		Providers:   providers.DefaultRegistry(),
		Store:       st,
	}
}

// Run executes the full pipeline:
//
//	read state -> extract expected -> fetch actual from cloud -> diff -> persist report
func (s *Scanner) Run(ctx context.Context, opts Options) (*model.DriftReport, error) {
	report := &model.DriftReport{
		ScanID:      uuid.New().String(),
		WorkspaceID: opts.WorkspaceID,
		Workspace:   opts.WorkspaceName,
		StartedAt:   time.Now().UTC(),
		Status:      model.ScanStatusRunning,
		Findings:    []model.DriftFinding{},
	}
	if err := s.Store.SaveScan(ctx, report); err != nil {
		return nil, fmt.Errorf("save scan: %w", err)
	}

	// 1. Expected state.
	stateCfg := opts.State
	if stateCfg.Endpoint == "" {
		stateCfg.Endpoint = opts.Endpoint
	}
	stateData, err := s.StateReader.Read(ctx, stateCfg)
	if err != nil {
		return s.failScan(ctx, report, fmt.Errorf("read state: %w", err))
	}
	expected, err := state.NewExtractor(opts.Provider).Extract(stateData)
	if err != nil {
		return s.failScan(ctx, report, fmt.Errorf("extract state: %w", err))
	}

	provider, ok := s.Providers.Get(opts.Provider)
	if !ok {
		return s.failScan(ctx, report, fmt.Errorf("unsupported provider: %s", opts.Provider))
	}
	expected, skipped := filterSupported(expected, provider.SupportedTypes())
	if len(skipped) > 0 {
		report.Errors = append(report.Errors, fmt.Sprintf("not checked (unsupported types): %s", strings.Join(skipped, ", ")))
	}

	// 2. Actual state.
	var actual []model.Resource
	if !opts.SkipCloud {
		var errs []error
		actual, errs = provider.FetchResources(ctx, providers.FetchRequest{
			Expected: expected,
			Regions:  opts.Regions,
			Endpoint: opts.Endpoint,
			AllTypes: opts.Compare.DetectUnmanaged,
		})
		for _, e := range errs {
			report.Errors = append(report.Errors, e.Error())
		}
		if len(errs) > 0 && len(actual) == 0 {
			// Nothing came back: comparing would wrongly flag everything as missing.
			return s.failScan(ctx, report, fmt.Errorf("cloud fetch failed: %v", errs[0]))
		}
	}

	// 3. Diff.
	compare := opts.Compare
	if opts.State.Backend == "s3" && opts.State.Bucket != "" {
		// The bucket holding the state is infrastructure *for* Terraform, not managed by it.
		compare.IgnoreResources = append(compare.IgnoreResources, opts.State.Bucket)
	}
	findings := drift.NewEngine(compare).Compare(expected, actual)

	report.CompletedAt = time.Now().UTC()
	report.Status = model.ScanStatusCompleted
	report.Summary = drift.BuildSummary(len(expected), findings)
	report.Findings = findings

	if err := s.Store.SaveScan(ctx, report); err != nil {
		return nil, fmt.Errorf("save completed scan: %w", err)
	}
	return report, nil
}

func filterSupported(in []model.Resource, supported []string) ([]model.Resource, []string) {
	ok := make(map[string]bool, len(supported))
	for _, t := range supported {
		ok[t] = true
	}
	var out []model.Resource
	seen := make(map[string]bool)
	var skipped []string
	for _, r := range in {
		if ok[r.Type] {
			out = append(out, r)
			continue
		}
		if !seen[r.Type] {
			seen[r.Type] = true
			skipped = append(skipped, r.Type)
		}
	}
	return out, skipped
}

func (s *Scanner) failScan(ctx context.Context, report *model.DriftReport, err error) (*model.DriftReport, error) {
	report.CompletedAt = time.Now().UTC()
	report.Status = model.ScanStatusFailed
	report.Errors = append(report.Errors, err.Error())
	_ = s.Store.SaveScan(ctx, report)
	return report, err
}
