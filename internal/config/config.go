package config

import (
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"

	"github.com/malekachouri/terraform-drift-detector/internal/model"
)

// File represents the driftctl.yaml configuration.
type File struct {
	Database   string            `yaml:"database"`
	API        APIConfig         `yaml:"api"`
	Workspaces []model.Workspace `yaml:"workspaces"`
}

// APIConfig holds server settings.
type APIConfig struct {
	Addr   string `yaml:"addr"`
	APIKey string `yaml:"api_key"`
}

// Load reads configuration from a YAML file. Environment variables are
// expanded before parsing using ${VAR} or ${VAR:-default}, so the same file
// works from the host (localhost:4566) and from docker compose (localstack:4566).
func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg File
	if err := yaml.Unmarshal([]byte(ExpandEnv(string(data))), &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if cfg.Database == "" {
		cfg.Database = "driftctl.db"
	}
	if cfg.API.Addr == "" {
		cfg.API.Addr = ":8080"
	}
	return &cfg, nil
}

var envPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?::-([^}]*))?\}`)

// ExpandEnv replaces ${VAR} and ${VAR:-default} with environment values.
func ExpandEnv(s string) string {
	return envPattern.ReplaceAllStringFunc(s, func(m string) string {
		parts := envPattern.FindStringSubmatch(m)
		if v, ok := os.LookupEnv(parts[1]); ok && v != "" {
			return v
		}
		return parts[2]
	})
}
