package drift

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"

	"github.com/malekachouri/terraform-drift-detector/internal/model"
)

// Engine compares expected (Terraform state) and actual (cloud) resource sets.
type Engine struct {
	IgnoreTags       map[string]bool
	IgnoreAttributes map[string]bool
	IgnoreResources  map[string]bool
	DetectUnmanaged  bool
}

func NewEngine(cfg model.CompareConfig) *Engine {
	e := &Engine{
		IgnoreTags:       toSet(cfg.IgnoreTags),
		IgnoreAttributes: toSet(cfg.IgnoreAttributes),
		IgnoreResources:  toSet(cfg.IgnoreResources),
		DetectUnmanaged:  cfg.DetectUnmanaged,
	}
	e.IgnoreTags["driftctl_scan"] = true
	return e
}

func toSet(items []string) map[string]bool {
	out := make(map[string]bool, len(items))
	for _, i := range items {
		out[i] = true
	}
	return out
}

// Compare joins both sides on Resource.ID and produces findings:
//
//	in state, not in cloud  -> missing_in_cloud   (deleted by hand)
//	in cloud, not in state  -> extra_in_cloud     (created by hand, if DetectUnmanaged)
//	in both, values differ  -> attribute_changed / tags_changed
func (e *Engine) Compare(expected, actual []model.Resource) []model.DriftFinding {
	actualIndex := make(map[string]model.Resource, len(actual))
	for _, r := range actual {
		actualIndex[r.ID] = r
	}

	expectedIndex := make(map[string]bool, len(expected))
	findings := []model.DriftFinding{}

	for _, exp := range expected {
		expectedIndex[exp.ID] = true
		act, ok := actualIndex[exp.ID]
		if !ok {
			findings = append(findings, e.finding(model.DriftMissingInCloud, exp, "", nil, nil, model.SeverityCritical))
			continue
		}
		findings = append(findings, e.diffAttributes(exp, act)...)
		findings = append(findings, e.diffTags(exp, act)...)
	}

	if e.DetectUnmanaged {
		for _, act := range actual {
			if expectedIndex[act.ID] || e.IgnoreResources[act.CloudID] {
				continue
			}
			findings = append(findings, e.finding(model.DriftExtraInCloud, act, "", nil, nil, model.SeverityWarning))
		}
	}

	sort.SliceStable(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if severityRank[a.Severity] != severityRank[b.Severity] {
			return severityRank[a.Severity] > severityRank[b.Severity]
		}
		if a.ResourceID != b.ResourceID {
			return a.ResourceID < b.ResourceID
		}
		return a.Field < b.Field
	})
	return findings
}

var severityRank = map[string]int{
	model.SeverityInfo:     0,
	model.SeverityWarning:  1,
	model.SeverityCritical: 2,
}

func (e *Engine) finding(kind model.DriftKind, r model.Resource, field string, expected, actual any, severity string) model.DriftFinding {
	return model.DriftFinding{
		Kind:         kind,
		ResourceID:   r.ID,
		ResourceType: r.Type,
		ResourceName: r.Name,
		Address:      r.Address,
		Field:        field,
		Expected:     expected,
		Actual:       actual,
		Severity:     severity,
	}
}

// criticalAttributes are security-relevant: a change there is escalated.
var criticalAttributes = map[string]bool{
	"ingress":                true,
	"egress":                 true,
	"vpc_security_group_ids": true,
}

func (e *Engine) diffAttributes(exp, act model.Resource) []model.DriftFinding {
	var findings []model.DriftFinding
	// Only attributes known from the state are compared; the cloud may expose more.
	for key, ev := range exp.Attributes {
		if e.IgnoreAttributes[key] || e.IgnoreAttributes[exp.Type+"."+key] {
			continue
		}
		av := act.Attributes[key]
		if valuesEqual(ev, av) {
			continue
		}
		severity := model.SeverityWarning
		if criticalAttributes[key] {
			severity = model.SeverityCritical
		}
		findings = append(findings, e.finding(model.DriftAttributeChange, exp, key, ev, av, severity))
	}
	return findings
}

func (e *Engine) diffTags(exp, act model.Resource) []model.DriftFinding {
	var findings []model.DriftFinding
	expTags := filterTags(exp.Tags, e.IgnoreTags)
	actTags := filterTags(act.Tags, e.IgnoreTags)

	keys := make(map[string]bool)
	for k := range expTags {
		keys[k] = true
	}
	for k := range actTags {
		keys[k] = true
	}

	for key := range keys {
		ev, eok := expTags[key]
		av, aok := actTags[key]
		if eok == aok && ev == av {
			continue
		}
		var expected, actual any
		if eok {
			expected = ev
		}
		if aok {
			actual = av
		}
		findings = append(findings, e.finding(model.DriftTagsChanged, exp, fmt.Sprintf("tags.%s", key), expected, actual, model.SeverityInfo))
	}
	return findings
}

func filterTags(tags map[string]string, ignore map[string]bool) map[string]string {
	out := make(map[string]string)
	for k, v := range tags {
		if !ignore[k] {
			out[k] = v
		}
	}
	return out
}

// valuesEqual compares via JSON so []string vs []any and int vs float64 match.
func valuesEqual(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return isEmpty(a) && isEmpty(b)
	}
	aj, err1 := json.Marshal(a)
	bj, err2 := json.Marshal(b)
	if err1 == nil && err2 == nil {
		return string(aj) == string(bj)
	}
	return reflect.DeepEqual(a, b)
}

func isEmpty(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice, reflect.Map, reflect.String:
		return rv.Len() == 0
	}
	return false
}

// BuildSummary computes summary statistics from findings and resource counts.
func BuildSummary(expectedCount int, findings []model.DriftFinding) model.DriftSummary {
	s := model.DriftSummary{
		TotalResources: expectedCount,
		TotalFindings:  len(findings),
	}
	for _, f := range findings {
		switch f.Kind {
		case model.DriftMissingInCloud:
			s.MissingInCloud++
		case model.DriftExtraInCloud:
			s.ExtraInCloud++
		case model.DriftAttributeChange:
			s.AttributeChanges++
		case model.DriftTagsChanged:
			s.TagChanges++
		}
	}
	return s
}
