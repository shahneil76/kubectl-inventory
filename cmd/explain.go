package cmd

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/shahneil76/kubectl-inventory/pkg/types"
	"github.com/spf13/cobra"
)

var explainCmd = &cobra.Command{
	Use:   "explain TYPE/NAME",
	Short: "Explain why a resource is classified the way it is",
	Long: `Explain shows a detailed classification report for a single resource.

It answers:
  - What is this resource's orphan/ownership status?
  - Why was it flagged suspicious or dangling?
  - Who owns it (ownerReferences)?
  - What other resources reference it?
  - Is it managed by a GitOps tool?
  - Does it have stuck finalizers?
  - How old is it, and who created it?

Examples:
  kubectl inventory explain pod/my-pod -n production
  kubectl inventory explain secret/my-secret -n staging
  kubectl inventory explain configmap/app-config -n default`,
	Args: cobra.ExactArgs(1),
	RunE: runExplain,
}

func init() {
	rootCmd.AddCommand(explainCmd)
}

func runExplain(cmd *cobra.Command, args []string) error {
	// Parse TYPE/NAME
	parts := strings.SplitN(args[0], "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("invalid resource format %q — expected TYPE/NAME (e.g. pod/my-pod)", args[0])
	}
	targetKind := normalizeKind(parts[0])
	targetName := parts[1]

	// Run a full scan (same as default) so we get the full relationship graph.
	inv, err := scanInventory(cmd.Context(), opts.Namespace, opts.AllNamespaces)
	if err != nil {
		return err
	}

	// Find the target resource.
	var target *types.Resource
	for i := range inv.Resources {
		r := &inv.Resources[i]
		if strings.EqualFold(r.Kind, targetKind) && r.Name == targetName {
			target = r
			break
		}
	}
	if target == nil {
		return fmt.Errorf("resource %s/%s not found in namespace %q\n(scanned %d resources across %d types)",
			targetKind, targetName, inv.Namespace, len(inv.Resources), inv.Stats.ScannedTypes)
	}

	// Build reverse-reference index: which resources reference this UID?
	referredBy := findReferrers(inv.Resources, target)

	// Build owner chain.
	ownerChain := buildOwnerChain(inv.Resources, target)

	return renderExplain(cmd.OutOrStdout(), target, referredBy, ownerChain, inv)
}

func renderExplain(w io.Writer, r *types.Resource, referredBy []types.Resource, ownerChain []types.Resource, inv types.Inventory) error {
	bold := color.New(color.Bold)
	dim := color.New(color.Faint)
	warn := color.New(color.FgYellow, color.Bold)
	good := color.New(color.FgGreen)
	bad := color.New(color.FgRed, color.Bold)

	sep := "─────────────────────────────────────────────────────────────"

	fmt.Fprintln(w, bold.Sprintf("%s/%s", r.Kind, r.Name))
	fmt.Fprintln(w, sep)
	fmt.Fprintln(w)

	// ── Identity ──────────────────────────────────────────────────────────────
	fmt.Fprintln(w, bold.Sprint("Identity"))
	if r.Namespace != "" {
		fmt.Fprintf(w, "  Namespace:  %s\n", r.Namespace)
	}
	gvr := r.Resource
	if r.Group != "" {
		gvr = r.Resource + "." + r.Group
	}
	fmt.Fprintf(w, "  GVR:        %s/%s/%s\n", r.Group, r.Version, r.Resource)
	fmt.Fprintf(w, "  Kind:       %s\n", r.Kind)
	fmt.Fprintf(w, "  UID:        %s\n", r.UID)
	fmt.Fprintf(w, "  Age:        %s\n", formatAge(r.Age))
	fmt.Fprintf(w, "  Created:    %s\n", r.CreatedAt.Format(time.RFC3339))
	if r.CreatedBy != "" {
		fmt.Fprintf(w, "  Manager:    %s\n", r.CreatedBy)
	}
	_ = gvr
	fmt.Fprintln(w)

	// ── Classification ────────────────────────────────────────────────────────
	fmt.Fprintln(w, bold.Sprint("Classification"))
	statusColor := good
	switch r.OrphanStatus {
	case types.OrphanStatusSuspicious:
		statusColor = warn
	case types.OrphanStatusDangling:
		statusColor = bad
	case types.OrphanStatusStandalone:
		statusColor = dim
	}
	fmt.Fprintf(w, "  Status:     %s\n", statusColor.Sprint(strings.ToUpper(coalesce(r.OrphanStatus, "unknown"))))
	if len(r.OrphanReasons) > 0 {
		fmt.Fprintln(w, "  Reasons:")
		for _, reason := range r.OrphanReasons {
			fmt.Fprintf(w, "    • %s\n", reason)
		}
	}
	fmt.Fprintln(w)

	// ── Controller / Release manager ─────────────────────────────────────────
	// Only show this section when a managing tool was detected.
	if r.GitOpsTool != "" {
		sectionLabel := "Controller / Release manager"
		fmt.Fprintln(w, bold.Sprint(sectionLabel))

		// Helm is a release manager, not a GitOps tool. ArgoCD and Flux are.
		var fieldLabel string
		switch r.GitOpsTool {
		case types.GitOpsHelm:
			fieldLabel = "Release mgr:"
		case types.GitOpsArgo, types.GitOpsFlux:
			fieldLabel = "GitOps:     "
		default:
			fieldLabel = "Managed by: "
		}
		fmt.Fprintf(w, "  %s %s\n", fieldLabel, good.Sprint(r.GitOpsTool))
		if r.AppName != "" {
			fmt.Fprintf(w, "  App:        %s\n", r.AppName)
		}
		if r.SourcePath != "" {
			fmt.Fprintf(w, "  Source:     %s\n", r.SourcePath)
		}
		fmt.Fprintln(w)
	}

	// ── Ownership ─────────────────────────────────────────────────────────────
	fmt.Fprintln(w, bold.Sprint("Ownership"))
	if len(r.OwnerRefs) == 0 {
		fmt.Fprintf(w, "  ownerReferences: %s\n", dim.Sprint("none"))
	} else {
		fmt.Fprintln(w, "  ownerReferences:")
		uidSet := buildUIDSetFrom(inv.Resources)
		for _, ref := range r.OwnerRefs {
			present := "✓ present in cluster"
			if _, ok := uidSet[string(ref.UID)]; !ok {
				present = bad.Sprint("✗ missing from cluster — DANGLING")
			}
			fmt.Fprintf(w, "    • %s/%s  [%s]  %s\n", ref.Kind, ref.Name, ref.UID, present)
		}
	}
	if len(ownerChain) > 0 {
		fmt.Fprintln(w, "  Owner chain (top → this):")
		for i, o := range ownerChain {
			indent := strings.Repeat("  ", i+2)
			fmt.Fprintf(w, "%s→ %s/%s\n", indent, o.Kind, o.Name)
		}
	}
	fmt.Fprintln(w)

	// ── References ────────────────────────────────────────────────────────────
	fmt.Fprintln(w, bold.Sprint("Referenced By"))
	if len(referredBy) == 0 {
		fmt.Fprintf(w, "  %s\n", dim.Sprint("not referenced by any scanned resource"))
	} else {
		sort.Slice(referredBy, func(i, j int) bool {
			return referredBy[i].Kind+referredBy[i].Name < referredBy[j].Kind+referredBy[j].Name
		})
		for _, ref := range referredBy {
			fmt.Fprintf(w, "  • %s/%s", ref.Kind, ref.Name)
			if ref.Namespace != "" && ref.Namespace != r.Namespace {
				fmt.Fprintf(w, " [%s]", ref.Namespace)
			}
			fmt.Fprintln(w)
		}
	}
	fmt.Fprintln(w)

	// ── Finalizers ────────────────────────────────────────────────────────────
	fmt.Fprintln(w, bold.Sprint("Finalizers"))
	if len(r.Finalizers) == 0 {
		fmt.Fprintf(w, "  %s\n", dim.Sprint("none"))
	} else {
		stuckLabel := ""
		if r.IsStuck {
			stuckLabel = " " + bad.Sprint("[STUCK — deletionTimestamp set]")
		}
		fmt.Fprintf(w, "  Count: %d%s\n", len(r.Finalizers), stuckLabel)
		for _, f := range r.Finalizers {
			fmt.Fprintf(w, "    • %s\n", f)
		}
	}
	fmt.Fprintln(w)

	// ── Context ───────────────────────────────────────────────────────────────
	fmt.Fprintln(w, dim.Sprintf("Cluster: %s  |  Namespace: %s  |  Scanned: %d resources",
		inv.Context, inv.Namespace, len(inv.Resources)))

	return nil
}

// findReferrers returns all resources that reference the target by UID (via
// IsReferenced signal) or by Kind+Name match in their OrphanReasons.
func findReferrers(resources []types.Resource, target *types.Resource) []types.Resource {
	var out []types.Resource
	targetUID := string(target.UID)
	seen := map[string]bool{}
	for _, r := range resources {
		if string(r.UID) == targetUID {
			continue
		}
		// Check if any of r's OrphanReasons mention the target.
		for _, reason := range r.OrphanReasons {
			if strings.Contains(reason, target.Kind+"/"+target.Name) ||
				strings.Contains(reason, target.Name) {
				key := string(r.UID)
				if !seen[key] {
					seen[key] = true
					out = append(out, r)
				}
			}
		}
		// Also check if target is referenced via the IsReferenced flag propagated
		// through the reference walker (available in --deep mode).
		for _, ref := range r.OwnerRefs {
			if string(ref.UID) == targetUID {
				key := string(r.UID)
				if !seen[key] {
					seen[key] = true
					out = append(out, r)
				}
			}
		}
	}
	return out
}

// buildOwnerChain walks up the ownerReference chain from target and returns
// the ancestors from root to immediate parent.
func buildOwnerChain(resources []types.Resource, target *types.Resource) []types.Resource {
	byUID := map[string]types.Resource{}
	for _, r := range resources {
		byUID[string(r.UID)] = r
	}

	var chain []types.Resource
	visited := map[string]bool{}
	current := target
	for {
		if len(current.OwnerRefs) == 0 {
			break
		}
		ownerUID := string(current.OwnerRefs[0].UID)
		if visited[ownerUID] {
			break // cycle guard
		}
		visited[ownerUID] = true
		owner, ok := byUID[ownerUID]
		if !ok {
			break
		}
		chain = append([]types.Resource{owner}, chain...) // prepend
		current = &owner
	}
	return chain
}

// buildUIDSetFrom builds a UID lookup set for dangling owner-ref detection.
func buildUIDSetFrom(resources []types.Resource) map[string]struct{} {
	set := make(map[string]struct{}, len(resources))
	for _, r := range resources {
		set[string(r.UID)] = struct{}{}
	}
	return set
}



// normalizeKind converts a short CLI form like "po", "deploy", "cm" to
// the canonical Kind name used in the inventory.
func normalizeKind(input string) string {
	lower := strings.ToLower(input)
	shortcuts := map[string]string{
		"po": "Pod", "pod": "Pod", "pods": "Pod",
		"deploy": "Deployment", "deployment": "Deployment", "deployments": "Deployment",
		"rs": "ReplicaSet", "replicaset": "ReplicaSet", "replicasets": "ReplicaSet",
		"sts": "StatefulSet", "statefulset": "StatefulSet", "statefulsets": "StatefulSet",
		"ds": "DaemonSet", "daemonset": "DaemonSet", "daemonsets": "DaemonSet",
		"svc": "Service", "service": "Service", "services": "Service",
		"cm": "ConfigMap", "configmap": "ConfigMap", "configmaps": "ConfigMap",
		"secret": "Secret", "secrets": "Secret",
		"sa": "ServiceAccount", "serviceaccount": "ServiceAccount",
		"ing": "Ingress", "ingress": "Ingress", "ingresses": "Ingress",
		"pvc": "PersistentVolumeClaim", "persistentvolumeclaim": "PersistentVolumeClaim",
		"pv": "PersistentVolume", "persistentvolume": "PersistentVolume",
		"job": "Job", "jobs": "Job",
		"cj": "CronJob", "cronjob": "CronJob", "cronjobs": "CronJob",
		"hpa": "HorizontalPodAutoscaler",
		"ep": "Endpoints", "endpoints": "Endpoints",
		"np": "NetworkPolicy", "networkpolicy": "NetworkPolicy",
		"rb": "RoleBinding", "rolebinding": "RoleBinding",
		"role": "Role", "roles": "Role",
	}
	if canonical, ok := shortcuts[lower]; ok {
		return canonical
	}
	// Title-case the first letter as a best-effort for CRD kinds.
	if len(input) > 0 {
		return strings.ToUpper(input[:1]) + input[1:]
	}
	return input
}

func formatAge(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func coalesce(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}


