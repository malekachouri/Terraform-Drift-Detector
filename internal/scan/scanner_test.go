package scan

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/malekachouri/terraform-drift-detector/internal/model"
	"github.com/malekachouri/terraform-drift-detector/internal/providers"
	"github.com/malekachouri/terraform-drift-detector/internal/state"
	"github.com/malekachouri/terraform-drift-detector/internal/store"
)

// fakeCloud returns exactly what the state says, plus the state bucket and one
// unmanaged bucket, so the only expected finding is the unmanaged bucket.
type fakeCloud struct{}

func (fakeCloud) Name() string { return "aws" }
func (fakeCloud) SupportedTypes() []string {
	return []string{"aws_instance", "aws_s3_bucket", "aws_security_group", "aws_subnet", "aws_vpc"}
}
func (fakeCloud) FetchResources(_ context.Context, req providers.FetchRequest) ([]model.Resource, []error) {
	out := make([]model.Resource, 0, len(req.Expected)+2)
	for _, r := range req.Expected {
		r.Source = "cloud"
		out = append(out, r)
	}
	for _, b := range []string{"tf-state", "shadow"} {
		out = append(out, model.Resource{ID: "aws/aws_s3_bucket/" + b, Type: "aws_s3_bucket", CloudID: b})
	}
	return out, nil
}

type fileAsS3 struct{ path string }

func (f fileAsS3) Read(context.Context, model.StateConfig) ([]byte, error) {
	return os.ReadFile(f.path)
}

func TestScannerRun(t *testing.T) {
	st, err := store.OpenSQLite(filepath.Join(t.TempDir(), "scan.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	reg := providers.NewRegistry()
	reg.Register(fakeCloud{})
	s := &Scanner{StateReader: fileAsS3{"../../testdata/state/sample.tfstate"}, Providers: reg, Store: st}

	report, err := s.Run(context.Background(), Options{
		WorkspaceName: "test",
		Provider:      "aws",
		State:         model.StateConfig{Backend: "s3", Bucket: "tf-state", Key: "k"},
		Compare:       model.CompareConfig{DetectUnmanaged: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != model.ScanStatusCompleted || report.Summary.TotalResources != 3 {
		t.Fatalf("unexpected report %+v", report)
	}
	// tf-state is auto-ignored; only "shadow" is unmanaged.
	if report.Summary.TotalFindings != 1 || report.Findings[0].ResourceID != "aws/aws_s3_bucket/shadow" {
		t.Fatalf("unexpected findings %+v", report.Findings)
	}

	saved, err := st.GetScan(context.Background(), report.ScanID)
	if err != nil || saved.Summary.TotalFindings != 1 {
		t.Fatalf("scan not persisted: %+v %v", saved, err)
	}
}

var _ state.Reader = fileAsS3{}
