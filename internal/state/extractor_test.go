package state

import (
	"os"
	"reflect"
	"testing"
)

func TestExtractorSample(t *testing.T) {
	data, err := os.ReadFile("../../testdata/state/sample.tfstate")
	if err != nil {
		t.Fatal(err)
	}
	resources, err := NewExtractor("aws").Extract(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 3 {
		t.Fatalf("expected 3 resources, got %d", len(resources))
	}
	for _, r := range resources {
		if r.Provider != "aws" || r.Source != "state" {
			t.Fatalf("unexpected resource %+v", r)
		}
	}
}

// localstack.tfstate is real output of `terraform apply` against LocalStack.
func TestExtractorLocalStackState(t *testing.T) {
	data, err := os.ReadFile("../../testdata/state/localstack.tfstate")
	if err != nil {
		t.Fatal(err)
	}
	resources, err := NewExtractor("").Extract(data)
	if err != nil {
		t.Fatal(err)
	}
	byAddr := map[string]int{}
	for i, r := range resources {
		byAddr[r.Address] = i
	}
	for _, addr := range []string{"aws_vpc.main", "aws_subnet.public", "aws_security_group.web",
		"aws_instance.web", "aws_s3_bucket.logs", "aws_s3_bucket.artifacts"} {
		if _, ok := byAddr[addr]; !ok {
			t.Fatalf("missing %s in %v", addr, byAddr)
		}
	}

	sg := resources[byAddr["aws_security_group.web"]]
	if got := sg.Attributes["ingress"]; !reflect.DeepEqual(got, []string{"tcp 80-80 0.0.0.0/0"}) {
		t.Errorf("ingress = %v", got)
	}
	if got := sg.Attributes["egress"]; !reflect.DeepEqual(got, []string{"all all 0.0.0.0/0"}) {
		t.Errorf("egress = %v", got)
	}

	inst := resources[byAddr["aws_instance.web"]]
	if inst.Region != "us-east-1" {
		t.Errorf("instance region from AZ = %q", inst.Region)
	}
	if inst.Tags["managed_by"] != "terraform" {
		t.Errorf("default_tags from tags_all not extracted: %v", inst.Tags)
	}

	bucket := resources[byAddr["aws_s3_bucket.logs"]]
	if bucket.CloudID != "drift-demo-logs" || bucket.Attributes["versioning_enabled"] != false {
		t.Errorf("bucket = %+v", bucket)
	}
	if _, ok := bucket.Attributes["force_destroy"]; ok {
		t.Error("terraform-only attribute force_destroy must not be compared")
	}
}

func TestRegionOf(t *testing.T) {
	cases := map[string]map[string]any{
		"eu-west-3":      {"region": "eu-west-3"},
		"us-east-1":      {"availability_zone": "us-east-1c"},
		"ap-southeast-2": {"availability_zone": "ap-southeast-2a"},
		"":               {},
	}
	for want, attrs := range cases {
		if got := regionOf(attrs); got != want {
			t.Errorf("regionOf(%v) = %q, want %q", attrs, got, want)
		}
	}
}

func TestExtractorRejectsOldState(t *testing.T) {
	if _, err := NewExtractor("aws").Extract([]byte(`{"version":3,"modules":[]}`)); err == nil {
		t.Fatal("expected error for state format v3")
	}
}
