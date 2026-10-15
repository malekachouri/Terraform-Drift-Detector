// Package canonical holds the shared, provider-neutral representations that
// both the Terraform state extractor and the cloud fetchers produce. Keeping
// them in one place guarantees a freshly-applied stack compares as "no drift".
package canonical

import (
	"fmt"
	"sort"
	"strings"
)

// SortedStrings returns a sorted copy without empty values.
func SortedStrings(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s != "" {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

// Protocol normalizes a security-group protocol ("-1", "all", "6", "TCP"...).
func Protocol(p string) string {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "-1", "all", "":
		return "all"
	case "6", "tcp":
		return "tcp"
	case "17", "udp":
		return "udp"
	case "1", "icmp":
		return "icmp"
	default:
		return strings.ToLower(p)
	}
}

// SGRule renders one security-group rule as a list of human readable strings,
// one per source, e.g. "tcp 80-80 from 0.0.0.0/0".
func SGRule(protocol string, from, to int64, sources []string) []string {
	proto := Protocol(protocol)
	ports := fmt.Sprintf("%d-%d", from, to)
	if proto == "all" {
		ports = "all"
	}
	out := make([]string, 0, len(sources))
	for _, s := range sources {
		out = append(out, fmt.Sprintf("%s %s %s", proto, ports, s))
	}
	return out
}
