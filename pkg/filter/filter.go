// Package filter provides filtering logic for API resources discovered during
// an inventory scan. It handles:
//   - Inclusion/exclusion of API groups and resource names
//   - The default "noise profile" that skips high-volume/low-value resources
//     (events, metrics, leases) unless the user explicitly opts in.
package filter

import (
	"strings"

	"github.com/shahneil76/kubectl-inventory/pkg/types"
)

// Options controls which API resources are included in a scan.
type Options struct {
	// Explicit inclusion filters (comma-separated lists, empty = no filter).
	APIGroups string
	Resources string

	// Explicit exclusion filters.
	ExcludeAPIGroups string
	ExcludeResources string

	// Noise-profile opt-in flags.
	IncludeEvents  bool
	IncludeMetrics bool
	IncludeNoisy   bool
}

// noiseProfile is the default set of resource names that are excluded unless
// the user opts in with --include-events / --include-metrics / --include-noisy.
// Keys are "<resource>.<group>"; for core resources the group is empty so the
// key is just "<resource>".
var noiseProfile = map[string]string{
	// Events — very high volume, rarely useful in a broad inventory.
	"events":              "events",
	"events.events.k8s.io": "events",
	// Metrics — aggregated API, often slow and not directly actionable.
	"podmetrics.metrics.k8s.io":  "metrics",
	"nodemetrics.metrics.k8s.io": "metrics",
	// Leases — heartbeat objects, not meaningfully informative in inventory.
	"leases.coordination.k8s.io": "noisy",
}

// noisyKinds are additional "generated/runtime" kinds skipped under --include-noisy.
// These are already handled by isKnownGeneratedResource in the analyzer, but excluding
// them at collection time saves unnecessary API calls.
var noisyKinds = map[string]bool{
	"CiliumEndpoint":    true,
	"CiliumNode":        true,
	"PodMetrics":        true,
	"NodeMetrics":       true,
	"Lease":             true,
	"Event":             true,
	"ControllerRevision": true,
}

// Apply filters the provided API resource list according to opts and returns
// the filtered slice plus the count of resources excluded by the noise profile.
func Apply(resources []types.APIResource, opts Options) ([]types.APIResource, int) {
	includeGroups := splitCSV(opts.APIGroups)
	excludeGroups := splitCSV(opts.ExcludeAPIGroups)
	includeResources := splitCSV(opts.Resources)
	excludeResources := splitCSV(opts.ExcludeResources)

	filtered := make([]types.APIResource, 0, len(resources))
	noisySkipped := 0

	for _, r := range resources {
		// ── User-driven exclusions (always respected) ─────────────────────────
		if len(excludeGroups) > 0 && containsCI(excludeGroups, r.Group) {
			continue
		}
		if len(excludeResources) > 0 && containsCI(excludeResources, r.Resource) {
			continue
		}

		// ── User-driven inclusions (restrict to these when set) ───────────────
		if len(includeGroups) > 0 && !containsCI(includeGroups, r.Group) {
			continue
		}
		if len(includeResources) > 0 && !containsCI(includeResources, r.Resource) {
			continue
		}

		// ── Noise profile ─────────────────────────────────────────────────────
		if !opts.IncludeNoisy {
			key := resourceKey(r)
			if category, isNoisy := noiseProfile[key]; isNoisy {
				if category == "events" && opts.IncludeEvents {
					// user opted in
				} else if category == "metrics" && opts.IncludeMetrics {
					// user opted in
				} else {
					noisySkipped++
					continue
				}
			}
			// Also skip known-noisy Kinds unless user opted in.
			if noisyKinds[r.Kind] && !isOptedIn(r, opts) {
				noisySkipped++
				continue
			}
		}

		filtered = append(filtered, r)
	}

	return filtered, noisySkipped
}

// isOptedIn returns true when the user has opted in via the appropriate flag
// for the given resource's noise category.
func isOptedIn(r types.APIResource, opts Options) bool {
	switch r.Kind {
	case "Event":
		return opts.IncludeEvents
	case "PodMetrics", "NodeMetrics":
		return opts.IncludeMetrics
	default:
		return false
	}
}

// resourceKey produces the canonical key used to look up a resource in the
// noiseProfile map: "<resource>.<group>" or just "<resource>" for core.
func resourceKey(r types.APIResource) string {
	if r.Group == "" {
		return r.Resource
	}
	return r.Resource + "." + r.Group
}

// splitCSV splits a comma-separated string into a trimmed, lower-cased slice.
// Returns nil when value is empty.
func splitCSV(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, strings.ToLower(p))
		}
	}
	return out
}

// containsCI performs a case-insensitive membership check.
func containsCI(slice []string, value string) bool {
	lower := strings.ToLower(value)
	for _, s := range slice {
		if s == lower {
			return true
		}
	}
	return false
}
