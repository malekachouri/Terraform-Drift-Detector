package state

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/malekachouri/terraform-drift-detector/internal/canonical"
	"github.com/malekachouri/terraform-drift-detector/internal/model"
)

type tfState struct {
	Version          int          `json:"version"`
	TerraformVersion string       `json:"terraform_version"`
	Resources        []tfResource `json:"resources"`
}

type tfResource struct {
	Module    string       `json:"module"`
	Mode      string       `json:"mode"`
	Type      string       `json:"type"`
	Name      string       `json:"name"`
	Provider  string       `json:"provider"`
	Instances []tfInstance `json:"instances"`
}

type tfInstance struct {
	IndexKey   any            `json:"index_key"`
	Attributes map[string]any `json:"attributes"`
}

// Extractor parses Terraform state JSON (format version 4) into normalized resources.
type Extractor struct {
	Provider string
}

func NewExtractor(provider string) *Extractor {
	return &Extractor{Provider: provider}
}

func (e *Extractor) Extract(data []byte) ([]model.Resource, error) {
	var st tfState
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("parse state JSON: %w", err)
	}
	if st.Version != 0 && st.Version < 4 {
		return nil, fmt.Errorf("unsupported state format version %d (need 4+)", st.Version)
	}

	var resources []model.Resource
	for _, res := range st.Resources {
		if res.Mode != "managed" { // skip data sources
			continue
		}
		for _, inst := range res.Instances {
			if r, ok := e.extractResource(res, inst); ok {
				resources = append(resources, r)
			}
		}
	}
	return resources, nil
}

func (e *Extractor) extractResource(res tfResource, inst tfInstance) (model.Resource, bool) {
	attrs := inst.Attributes
	if attrs == nil {
		return model.Resource{}, false
	}

	cloudID := resolveCloudID(res.Type, attrs)
	if cloudID == "" {
		return model.Resource{}, false
	}

	provider := e.Provider
	if provider == "" {
		provider = inferProvider(res.Provider, res.Type)
	}

	name := stringAttr(attrs, "name", "bucket")
	if t := extractTags(attrs); t["Name"] != "" {
		name = t["Name"]
	}
	if name == "" {
		name = cloudID
	}

	return model.Resource{
		ID:         fmt.Sprintf("%s/%s/%s", provider, res.Type, cloudID),
		Provider:   provider,
		Type:       res.Type,
		CloudID:    cloudID,
		Name:       name,
		Attributes: NormalizeAttributes(res.Type, attrs),
		Tags:       extractTags(attrs),
		Region:     regionOf(attrs),
		Source:     "state",
		Module:     strings.TrimPrefix(res.Module, "module."),
		Address:    address(res, inst),
	}, true
}

func address(res tfResource, inst tfInstance) string {
	addr := res.Type + "." + res.Name
	if res.Module != "" {
		addr = res.Module + "." + addr
	}
	switch k := inst.IndexKey.(type) {
	case float64:
		addr += fmt.Sprintf("[%d]", int64(k))
	case string:
		addr += fmt.Sprintf("[%q]", k)
	}
	return addr
}

var azSuffix = regexp.MustCompile(`^([a-z]{2}(-[a-z]+)+-\d+)[a-z]$`)

// regionOf derives the region: explicit "region" attribute (S3, provider v6),
// otherwise strip the zone letter from availability_zone (us-east-1a -> us-east-1).
func regionOf(attrs map[string]any) string {
	if r := stringAttr(attrs, "region"); r != "" {
		return r
	}
	if az := stringAttr(attrs, "availability_zone"); az != "" {
		if m := azSuffix.FindStringSubmatch(az); m != nil {
			return m[1]
		}
	}
	return ""
}

func resolveCloudID(resType string, attrs map[string]any) string {
	candidates := []string{"id", "arn", "bucket", "name"}
	if resType == "aws_s3_bucket" {
		candidates = []string{"bucket", "id"}
	}
	for _, key := range candidates {
		if v := stringAttr(attrs, key); v != "" {
			return v
		}
	}
	return ""
}

func stringAttr(attrs map[string]any, keys ...string) string {
	for _, key := range keys {
		if s, ok := attrs[key].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func boolAttr(attrs map[string]any, key string) bool {
	b, _ := attrs[key].(bool)
	return b
}

func intAttr(m map[string]any, key string) int64 {
	f, _ := m[key].(float64)
	return int64(f)
}

func stringList(v any) []string {
	items, _ := v.([]any)
	out := make([]string, 0, len(items))
	for _, it := range items {
		if s, ok := it.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// extractTags prefers tags_all (includes provider default_tags, which is what
// actually lands on the cloud resource) and falls back to tags.
func extractTags(attrs map[string]any) map[string]string {
	tags := make(map[string]string)
	for _, key := range []string{"tags", "tags_all"} {
		if m, ok := attrs[key].(map[string]any); ok {
			for k, v := range m {
				tags[k] = fmt.Sprintf("%v", v)
			}
		}
	}
	return tags
}

func inferProvider(providerField, resType string) string {
	switch {
	case strings.Contains(providerField, "hashicorp/aws") || strings.HasPrefix(resType, "aws_"):
		return "aws"
	case strings.Contains(providerField, "azurerm") || strings.HasPrefix(resType, "azurerm_"):
		return "azure"
	case strings.Contains(providerField, "google") || strings.HasPrefix(resType, "google_"):
		return "gcp"
	}
	return "unknown"
}

// NormalizeAttributes keeps only the attributes driftctl knows how to read
// back from the cloud, converted to canonical form. Terraform-only settings
// (force_destroy, timeouts, ...) are dropped because the cloud never reports them.
func NormalizeAttributes(resType string, a map[string]any) map[string]any {
	switch resType {
	case "aws_instance":
		return map[string]any{
			"instance_type":          stringAttr(a, "instance_type"),
			"ami":                    stringAttr(a, "ami"),
			"subnet_id":              stringAttr(a, "subnet_id"),
			"vpc_security_group_ids": canonical.SortedStrings(stringList(a["vpc_security_group_ids"])),
			"monitoring":             boolAttr(a, "monitoring"),
		}
	case "aws_vpc":
		return map[string]any{
			"cidr_block":           stringAttr(a, "cidr_block"),
			"enable_dns_hostnames": boolAttr(a, "enable_dns_hostnames"),
			"enable_dns_support":   boolAttr(a, "enable_dns_support"),
			"instance_tenancy":     stringAttr(a, "instance_tenancy"),
		}
	case "aws_subnet":
		return map[string]any{
			"cidr_block":              stringAttr(a, "cidr_block"),
			"vpc_id":                  stringAttr(a, "vpc_id"),
			"map_public_ip_on_launch": boolAttr(a, "map_public_ip_on_launch"),
			"availability_zone":       stringAttr(a, "availability_zone"),
		}
	case "aws_security_group":
		return map[string]any{
			"description": stringAttr(a, "description"),
			"vpc_id":      stringAttr(a, "vpc_id"),
			"ingress":     sgRules(a["ingress"], stringAttr(a, "id")),
			"egress":      sgRules(a["egress"], stringAttr(a, "id")),
		}
	case "aws_s3_bucket":
		enabled := false
		if v, ok := a["versioning"].([]any); ok && len(v) > 0 {
			if m, ok := v[0].(map[string]any); ok {
				enabled = boolAttr(m, "enabled")
			}
		}
		return map[string]any{"versioning_enabled": enabled}
	}
	return map[string]any{}
}

func sgRules(v any, selfID string) []string {
	items, _ := v.([]any)
	var out []string
	for _, it := range items {
		rule, ok := it.(map[string]any)
		if !ok {
			continue
		}
		var sources []string
		sources = append(sources, stringList(rule["cidr_blocks"])...)
		sources = append(sources, stringList(rule["ipv6_cidr_blocks"])...)
		sources = append(sources, stringList(rule["prefix_list_ids"])...)
		for _, sg := range stringList(rule["security_groups"]) {
			if sg == selfID {
				sg = "self"
			}
			sources = append(sources, sg)
		}
		if boolAttr(rule, "self") {
			sources = append(sources, "self")
		}
		out = append(out, canonical.SGRule(stringAttr(rule, "protocol"),
			intAttr(rule, "from_port"), intAttr(rule, "to_port"), sources)...)
	}
	return canonical.SortedStrings(out)
}
