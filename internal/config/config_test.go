package config

import "testing"

func TestExpandEnv(t *testing.T) {
	t.Setenv("DRIFT_TEST_SET", "http://localstack:4566")
	cases := map[string]string{
		"${DRIFT_TEST_SET:-http://localhost:4566}":   "http://localstack:4566",
		"${DRIFT_TEST_UNSET:-http://localhost:4566}": "http://localhost:4566",
		"${DRIFT_TEST_UNSET}":                        "",
		"plain $NOT_EXPANDED":                        "plain $NOT_EXPANDED",
	}
	for in, want := range cases {
		if got := ExpandEnv(in); got != want {
			t.Errorf("ExpandEnv(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLoadRepoConfig(t *testing.T) {
	cfg, err := Load("../../configs/driftctl.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Workspaces) != 1 || cfg.Workspaces[0].Endpoint == "" || !cfg.Workspaces[0].Compare.DetectUnmanaged {
		t.Fatalf("unexpected config %+v", cfg.Workspaces)
	}
}
