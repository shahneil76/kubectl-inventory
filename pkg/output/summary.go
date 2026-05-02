package output

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/fatih/color"
	"github.com/shahneil76/kubectl-inventory/pkg/analyzer"
	"github.com/shahneil76/kubectl-inventory/pkg/types"
)

// wellKnownSections defines the fixed top sections and which Kinds belong to them.
// Any resource whose Kind appears here is shown in its named section and excluded
// from the dynamic API-group section below.
var wellKnownSections = []struct {
	Title string
	Kinds []string
}{
	{"Workloads", []string{"Deployment", "StatefulSet", "DaemonSet", "ReplicaSet", "Job", "CronJob", "Pod"}},
	{"Networking", []string{"Service", "Ingress", "NetworkPolicy", "EndpointSlice", "Endpoints", "Gateway"}},
	{"Configuration", []string{"ConfigMap", "Secret"}},
	{"Storage", []string{"PersistentVolumeClaim", "PersistentVolume", "StorageClass"}},
	{"Security", []string{"ServiceAccount", "Role", "RoleBinding", "ClusterRole", "ClusterRoleBinding"}},
	{"Autoscaling", []string{"HorizontalPodAutoscaler", "VerticalPodAutoscaler", "PodDisruptionBudget", "PodAutoscaler"}},
}

type kindStats struct {
	Count      int
	Dangling   int
	Suspicious int
}

func RenderSummary(w io.Writer, inv types.Inventory) error {
	stuck := analyzer.StuckResources(inv.Resources)

	// Per-kind stats for well-known section rendering.
	wellKnown := map[string]*kindStats{}
	// seenKinds tracks which Kinds have already been shown in a well-known section.
	seenKinds := map[string]bool{}
	for _, s := range wellKnownSections {
		for _, k := range s.Kinds {
			seenKinds[k] = true
			wellKnown[k] = &kindStats{}
		}
	}

	// Per-API-group, per-kind stats for the dynamic section.
	// key: API group (empty string = "core"), value: map[kind]*kindStats
	groupStats := map[string]map[string]*kindStats{}

	for _, resource := range inv.Resources {
		if seenKinds[resource.Kind] {
			ks := wellKnown[resource.Kind]
			ks.Count++
			if resource.OrphanStatus == types.OrphanStatusDangling {
				ks.Dangling++
			} else if resource.OrphanStatus == types.OrphanStatusSuspicious {
				ks.Suspicious++
			}
		} else {
			group := resource.Group
			if group == "" {
				group = "core"
			}
			if groupStats[group] == nil {
				groupStats[group] = map[string]*kindStats{}
			}
			if groupStats[group][resource.Kind] == nil {
				groupStats[group][resource.Kind] = &kindStats{}
			}
			ks := groupStats[group][resource.Kind]
			ks.Count++
			if resource.OrphanStatus == types.OrphanStatusDangling {
				ks.Dangling++
			} else if resource.OrphanStatus == types.OrphanStatusSuspicious {
				ks.Suspicious++
			}
		}
	}

	// Header
	header := "Namespace: " + inv.Namespace
	if inv.AllNamespaces {
		header = "Namespace: all"
	}
	if inv.Context != "" {
		header += "   Context: " + inv.Context
	}
	fmt.Fprintln(w, color.New(color.Bold).Sprint(header))
	fmt.Fprintln(w, "─────────────────────────────────────────────────────────────")
	fmt.Fprintln(w)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	// ── Well-known sections ──────────────────────────────────────────────────
	for _, section := range wellKnownSections {
		printed := false
		for _, kind := range section.Kinds {
			ks := wellKnown[kind]
			if ks.Count == 0 {
				continue
			}
			if !printed {
				fmt.Fprintln(tw, color.New(color.Bold).Sprint(section.Title))
				printed = true
			}
			fmt.Fprintf(tw, "  %s\t%d\t%s\n", kind, ks.Count, kindNote(ks))
		}
		if printed {
			fmt.Fprintln(tw)
		}
	}

	// ── Dynamic API-group sections ────────────────────────────────────────────
	// Sort groups so output is deterministic. "core" goes last (least interesting).
	groups := make([]string, 0, len(groupStats))
	for g := range groupStats {
		groups = append(groups, g)
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i] == "core" {
			return false
		}
		if groups[j] == "core" {
			return true
		}
		return groups[i] < groups[j]
	})

	for _, group := range groups {
		kindsInGroup := groupStats[group]
		// Sort kinds alphabetically.
		kinds := make([]string, 0, len(kindsInGroup))
		for k := range kindsInGroup {
			kinds = append(kinds, k)
		}
		sort.Strings(kinds)

		// Use a descriptive label. Raw group names are already meaningful
		// (e.g. kafka.strimzi.io, serving.knative.dev).
		label := group
		if group == "core" {
			label = "core (built-in)"
		}
		fmt.Fprintln(tw, color.New(color.Bold).Sprint(label))
		for _, kind := range kinds {
			ks := kindsInGroup[kind]
			fmt.Fprintf(tw, "  %s\t%d\t%s\n", kind, ks.Count, kindNote(ks))
		}
		fmt.Fprintln(tw)
	}

	// ── Stuck resources ───────────────────────────────────────────────────────
	fmt.Fprintln(tw, color.New(color.Bold).Sprint("Stuck Resources"))
	if len(stuck) == 0 {
		fmt.Fprintln(tw, "  None\t✓\t")
	} else {
		for _, resource := range stuck {
			fmt.Fprintf(tw, "  %s/%s\t%s\t%v\n", resource.Kind, resource.Name, resource.Namespace, resource.Finalizers)
		}
	}
	fmt.Fprintln(tw)

	fmt.Fprintf(tw, "Total resources:\t%d\t\n", len(inv.Resources))
	fmt.Fprintf(tw, "Scanned:\t%d/%d resource types\t(%d skipped by errors)\n", inv.Stats.ScannedTypes, inv.Stats.DiscoveredTypes, inv.Stats.SkippedTypes)
	if inv.Stats.NoisySkipped > 0 {
		fmt.Fprintf(tw, "Noise profile:\t%d resource types excluded\t(use --include-noisy to scan all)\n", inv.Stats.NoisySkipped)
	}
	fmt.Fprintf(tw, "Duration:\t%s\t\n", inv.Stats.Duration.Round(10_000_000))
	return tw.Flush()
}

func kindNote(ks *kindStats) string {
	if ks.Dangling == 0 && ks.Suspicious == 0 {
		return ""
	}
	parts := []string{}
	if ks.Dangling > 0 {
		parts = append(parts, fmt.Sprintf("dangling: %d", ks.Dangling))
	}
	if ks.Suspicious > 0 {
		parts = append(parts, fmt.Sprintf("suspicious: %d", ks.Suspicious))
	}
	return "(" + strings.Join(parts, ", ") + " ⚠)"
}
