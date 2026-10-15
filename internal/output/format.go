package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/malekachouri/terraform-drift-detector/internal/model"
)

// Format renders a drift report as json or table.
func Format(w io.Writer, report *model.DriftReport, format string) error {
	switch strings.ToLower(format) {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	case "table", "":
		return writeTable(w, report)
	default:
		return fmt.Errorf("unsupported output format: %s", format)
	}
}

func writeTable(w io.Writer, report *model.DriftReport) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Scan ID:\t%s\n", report.ScanID)
	fmt.Fprintf(tw, "Workspace:\t%s\n", report.Workspace)
	fmt.Fprintf(tw, "Status:\t%s\n", report.Status)
	fmt.Fprintf(tw, "Started:\t%s\n", report.StartedAt.Format("2006-01-02 15:04:05 UTC"))
	if !report.CompletedAt.IsZero() {
		fmt.Fprintf(tw, "Duration:\t%s\n", report.CompletedAt.Sub(report.StartedAt).Round(1e6))
	}
	fmt.Fprintln(tw, "")
	fmt.Fprintln(tw, "SUMMARY")
	fmt.Fprintf(tw, "Resources in state:\t%d\n", report.Summary.TotalResources)
	fmt.Fprintf(tw, "Missing in cloud:\t%d\n", report.Summary.MissingInCloud)
	fmt.Fprintf(tw, "Unmanaged in cloud:\t%d\n", report.Summary.ExtraInCloud)
	fmt.Fprintf(tw, "Attribute changes:\t%d\n", report.Summary.AttributeChanges)
	fmt.Fprintf(tw, "Tag changes:\t%d\n", report.Summary.TagChanges)
	fmt.Fprintf(tw, "Total findings:\t%d\n", report.Summary.TotalFindings)
	fmt.Fprintln(tw, "")

	for _, e := range report.Errors {
		fmt.Fprintf(tw, "NOTE:\t%s\n", e)
	}
	if len(report.Errors) > 0 {
		fmt.Fprintln(tw, "")
	}

	if report.Status != model.ScanStatusCompleted {
		return tw.Flush()
	}
	if len(report.Findings) == 0 {
		fmt.Fprintln(tw, "No drift detected. Cloud matches Terraform state.")
		return tw.Flush()
	}

	fmt.Fprintln(tw, "FINDINGS")
	fmt.Fprintln(tw, "SEVERITY\tKIND\tRESOURCE\tFIELD\tEXPECTED (state)\tACTUAL (cloud)")
	for _, f := range report.Findings {
		resource := f.Address
		if resource == "" {
			resource = fmt.Sprintf("%s.%s", f.ResourceType, f.ResourceName)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			strings.ToUpper(f.Severity), f.Kind, resource, dash(f.Field), render(f.Expected), render(f.Actual))
	}
	return tw.Flush()
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func render(v any) string {
	switch t := v.(type) {
	case nil:
		return "-"
	case string:
		return t
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return fmt.Sprintf("%v", t)
		}
		return string(b)
	}
}

// HasDrift returns true if the report contains actionable drift.
func HasDrift(report *model.DriftReport) bool {
	return report.Summary.TotalFindings > 0
}
