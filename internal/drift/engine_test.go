package drift

import (
	"testing"

	"github.com/malekachouri/terraform-drift-detector/internal/model"
)

func res(id, typ string, attrs map[string]any, tags map[string]string) model.Resource {
	return model.Resource{ID: "aws/" + typ + "/" + id, Type: typ, CloudID: id, Name: id, Attributes: attrs, Tags: tags}
}

func TestCompareAllKinds(t *testing.T) {
	expected := []model.Resource{
		res("i-1", "aws_instance", map[string]any{"instance_type": "t3.micro", "vpc_security_group_ids": []string{"sg-1"}}, map[string]string{"env": "dev"}),
		res("gone", "aws_s3_bucket", map[string]any{"versioning_enabled": false}, nil),
	}
	actual := []model.Resource{
		res("i-1", "aws_instance", map[string]any{"instance_type": "t3.large", "vpc_security_group_ids": []any{"sg-1"}, "extra_cloud_only": 1}, map[string]string{"env": "prod", "owner": "bob"}),
		res("shadow", "aws_s3_bucket", nil, nil),
		res("tf-state", "aws_s3_bucket", nil, nil),
	}

	engine := NewEngine(model.CompareConfig{DetectUnmanaged: true, IgnoreResources: []string{"tf-state"}})
	findings := engine.Compare(expected, actual)
	s := BuildSummary(len(expected), findings)

	if s.MissingInCloud != 1 || s.ExtraInCloud != 1 || s.AttributeChanges != 1 || s.TagChanges != 2 {
		t.Fatalf("unexpected summary %+v\n%+v", s, findings)
	}
	if findings[0].Severity != model.SeverityCritical || findings[0].Kind != model.DriftMissingInCloud {
		t.Fatalf("findings must be sorted by severity, got %+v", findings[0])
	}
}

func TestCompareNoDriftAcrossTypes(t *testing.T) {
	// []string from state vs []any from JSON, int vs float64 must compare equal.
	exp := []model.Resource{res("sg-1", "aws_security_group", map[string]any{"ingress": []string{"tcp 80-80 0.0.0.0/0"}, "port": int64(80)}, nil)}
	act := []model.Resource{res("sg-1", "aws_security_group", map[string]any{"ingress": []any{"tcp 80-80 0.0.0.0/0"}, "port": float64(80)}, nil)}
	if f := NewEngine(model.CompareConfig{}).Compare(exp, act); len(f) != 0 {
		t.Fatalf("expected no drift, got %+v", f)
	}
}

func TestUnmanagedOnlyWhenEnabled(t *testing.T) {
	act := []model.Resource{res("shadow", "aws_s3_bucket", nil, nil)}
	if f := NewEngine(model.CompareConfig{}).Compare(nil, act); len(f) != 0 {
		t.Fatalf("extra_in_cloud must be opt-in, got %+v", f)
	}
}

func TestIgnoreRules(t *testing.T) {
	exp := []model.Resource{res("i-1", "aws_instance", map[string]any{"ami": "a", "instance_type": "x"}, map[string]string{"build": "1"})}
	act := []model.Resource{res("i-1", "aws_instance", map[string]any{"ami": "b", "instance_type": "y"}, map[string]string{"build": "2"})}
	e := NewEngine(model.CompareConfig{IgnoreAttributes: []string{"ami", "aws_instance.instance_type"}, IgnoreTags: []string{"build"}})
	if f := e.Compare(exp, act); len(f) != 0 {
		t.Fatalf("expected everything ignored, got %+v", f)
	}
}
