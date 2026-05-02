package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/shahneil76/kubectl-inventory/pkg/analyzer"
	"github.com/shahneil76/kubectl-inventory/pkg/client"
	"github.com/shahneil76/kubectl-inventory/pkg/collector"
	invdiscovery "github.com/shahneil76/kubectl-inventory/pkg/discovery"
	"github.com/shahneil76/kubectl-inventory/pkg/filter"
	"github.com/shahneil76/kubectl-inventory/pkg/output"
	"github.com/shahneil76/kubectl-inventory/pkg/types"
	"github.com/spf13/cobra"
)

type globalOptions struct {
	// Targeting
	Namespace     string
	AllNamespaces bool
	IncludeSystem bool

	// Output
	Output  string
	NoColor bool

	// Kubeconfig
	Kubeconfig string
	Context    string

	// Age filter (orphan analysis)
	AgeRaw string
	Age    time.Duration

	// ── Phase-1 flags ────────────────────────────────────────────────────────

	// Resource/API-group filters
	APIGroups        string
	ExcludeAPIGroups string
	Resources        string
	ExcludeResources string

	// Noise profile opt-ins
	IncludeEvents  bool
	IncludeMetrics bool
	IncludeNoisy   bool

	// Label selector
	Selector string

	// Concurrency
	Concurrency int

	// Per-GVR request timeout
	RequestTimeoutRaw string
	RequestTimeout    time.Duration

	// ── Phase-2 flags ────────────────────────────────────────────────────────

	// Deep mode: fetch full objects for all resource types (slower, more refs)
	Deep bool
}

var opts globalOptions

var rootCmd = &cobra.Command{
	Use:   "kubectl-inventory",
	Short: "Complete Kubernetes namespace and cluster resource inventory",
	Long:  "kubectl-inventory gives you a complete picture of every resource, including CRDs, suspicious orphan candidates, and stuck finalizers.",
	RunE:  runScan,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// ── Core flags ───────────────────────────────────────────────────────────
	rootCmd.PersistentFlags().StringVarP(&opts.Namespace, "namespace", "n", "", "Target namespace (default: current context namespace)")
	rootCmd.PersistentFlags().BoolVarP(&opts.AllNamespaces, "all-namespaces", "A", false, "Scan all namespaces")
	rootCmd.PersistentFlags().StringVarP(&opts.Output, "output", "o", "table", "Output format: table, json, tree, wide")
	rootCmd.PersistentFlags().StringVar(&opts.Kubeconfig, "kubeconfig", "", "Path to kubeconfig")
	rootCmd.PersistentFlags().StringVar(&opts.Context, "context", "", "Kubeconfig context to use")
	rootCmd.PersistentFlags().BoolVar(&opts.IncludeSystem, "include-system", false, "Include kube-system and other system namespaces")
	rootCmd.PersistentFlags().StringVar(&opts.AgeRaw, "age", "", "Filter orphan resources older than duration (e.g. 30d, 7d, 24h)")
	rootCmd.PersistentFlags().BoolVar(&opts.NoColor, "no-color", false, "Disable color output")

	// ── Phase-1: Resource / API-group filters ────────────────────────────────
	rootCmd.PersistentFlags().StringVar(&opts.APIGroups, "api-groups", "", "Comma-separated API groups to scan (e.g. apps,batch,networking.k8s.io)")
	rootCmd.PersistentFlags().StringVar(&opts.ExcludeAPIGroups, "exclude-api-groups", "", "Comma-separated API groups to exclude")
	rootCmd.PersistentFlags().StringVar(&opts.Resources, "resources", "", "Comma-separated resource names to scan (e.g. pods,secrets,deployments)")
	rootCmd.PersistentFlags().StringVar(&opts.ExcludeResources, "exclude-resources", "", "Comma-separated resource names to exclude")

	// ── Phase-1: Noise-profile opt-in flags ──────────────────────────────────
	rootCmd.PersistentFlags().BoolVar(&opts.IncludeEvents, "include-events", false, "Include events (excluded by default noise profile)")
	rootCmd.PersistentFlags().BoolVar(&opts.IncludeMetrics, "include-metrics", false, "Include metrics resources (excluded by default noise profile)")
	rootCmd.PersistentFlags().BoolVar(&opts.IncludeNoisy, "include-noisy", false, "Include all resources excluded by the default noise profile")

	// ── Phase-1: Selector ────────────────────────────────────────────────────
	rootCmd.PersistentFlags().StringVarP(&opts.Selector, "selector", "l", "", "Label selector to filter resources (passed to every List call)")

	// ── Phase-1: Concurrency ─────────────────────────────────────────────────
	rootCmd.PersistentFlags().IntVar(&opts.Concurrency, "concurrency", 10, "Max concurrent API list calls (increase for faster scans on large clusters)")

	// ── Phase-1: Per-request timeout ─────────────────────────────────────────
	rootCmd.PersistentFlags().StringVar(&opts.RequestTimeoutRaw, "request-timeout", "30s", "Per-GVR request timeout (e.g. 5s, 2s); 0 = no timeout")

	// ── Phase-2: Deep mode ────────────────────────────────────────────────────
	rootCmd.PersistentFlags().BoolVar(&opts.Deep, "deep", false, "Fetch full object specs for all resource types (enables complete reference analysis; slower)")
}

func runScan(cmd *cobra.Command, args []string) error {
	inv, err := scanInventory(cmd.Context(), opts.Namespace, opts.AllNamespaces)
	if err != nil {
		return err
	}
	return renderInventory(cmd, inv)
}

func scanInventory(ctx context.Context, namespace string, allNamespaces bool) (types.Inventory, error) {
	if opts.NoColor {
		color.NoColor = true
	}

	age, err := parseAge(opts.AgeRaw)
	if err != nil {
		return types.Inventory{}, err
	}
	opts.Age = age

	reqTimeout, err := parseRequestTimeout(opts.RequestTimeoutRaw)
	if err != nil {
		return types.Inventory{}, fmt.Errorf("invalid --request-timeout %q: %w", opts.RequestTimeoutRaw, err)
	}
	opts.RequestTimeout = reqTimeout

	clients, err := client.New(client.Options{Kubeconfig: opts.Kubeconfig, Context: opts.Context})
	if err != nil {
		return types.Inventory{}, err
	}
	if namespace == "" {
		namespace = clients.Namespace
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	if opts.Output != "json" {
		s.Suffix = " scanning resources"
		s.Start()
		defer s.Stop()
	}

	// ── Discovery ─────────────────────────────────────────────────────────────
	apiResources, warnings, err := invdiscovery.DiscoverAllResources(clients.Discovery)
	if err != nil {
		return types.Inventory{}, fmt.Errorf("discover resources: %w", err)
	}

	// ── Scope filter (namespaced vs cluster-scoped) ───────────────────────────
	apiResources = scopeFilter(apiResources, allNamespaces)

	// ── Apply user filters + noise profile ───────────────────────────────────
	filterOpts := filter.Options{
		APIGroups:        opts.APIGroups,
		ExcludeAPIGroups: opts.ExcludeAPIGroups,
		Resources:        opts.Resources,
		ExcludeResources: opts.ExcludeResources,
		IncludeEvents:    opts.IncludeEvents,
		IncludeMetrics:   opts.IncludeMetrics,
		IncludeNoisy:     opts.IncludeNoisy,
	}
	discovered := len(apiResources)
	apiResources, noisySkipped := filter.Apply(apiResources, filterOpts)

	// ── Collection ────────────────────────────────────────────────────────────
	collectOpts := collector.Options{
		Namespace:      namespace,
		AllNamespaces:  allNamespaces,
		IncludeSystem:  opts.IncludeSystem,
		Age:            opts.Age,
		Concurrency:    opts.Concurrency,
		RequestTimeout: opts.RequestTimeout,
		Selector:       opts.Selector,
		Deep:           opts.Deep,
	}
	result, err := collector.CollectAll(ctx, clients.Dynamic, clients.Metadata, namespace, allNamespaces, apiResources, collectOpts)
	if err != nil {
		return types.Inventory{}, err
	}

	if !opts.IncludeSystem {
		result.Resources = filterSystemNamespaces(result.Resources)
	}
	analyzer.MarkOrphans(result.Resources, opts.Age)

	for _, warning := range warnings {
		result.Stats.SkippedReasons = append(result.Stats.SkippedReasons, warning)
	}

	// Patch stats: DiscoveredTypes is pre-noise-filter; NoisySkipped is noise-only.
	result.Stats.DiscoveredTypes = discovered + noisySkipped
	result.Stats.NoisySkipped = noisySkipped

	return types.Inventory{
		Context:       clients.Context,
		Namespace:     namespace,
		AllNamespaces: allNamespaces,
		Resources:     result.Resources,
		Stats:         result.Stats,
	}, nil
}

func renderInventory(cmd *cobra.Command, inv types.Inventory) error {
	switch strings.ToLower(opts.Output) {
	case "json":
		return output.RenderJSON(cmd.OutOrStdout(), inv)
	case "tree":
		return output.RenderTree(cmd.OutOrStdout(), inv)
	case "table", "wide", "":
		return output.RenderSummary(cmd.OutOrStdout(), inv)
	default:
		return fmt.Errorf("unsupported output format %q", opts.Output)
	}
}

func parseAge(value string) (time.Duration, error) {
	if value == "" {
		return 0, nil
	}
	if strings.HasSuffix(value, "d") {
		days := strings.TrimSuffix(value, "d")
		var n int
		if _, err := fmt.Sscanf(days, "%d", &n); err != nil {
			return 0, fmt.Errorf("invalid age %q", value)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	return time.ParseDuration(value)
}

func parseRequestTimeout(value string) (time.Duration, error) {
	if value == "" || value == "0" || value == "0s" {
		return 0, nil
	}
	return time.ParseDuration(value)
}

// scopeFilter removes cluster-scoped resources when not scanning all namespaces.
func scopeFilter(resources []types.APIResource, allNamespaces bool) []types.APIResource {
	filtered := make([]types.APIResource, 0, len(resources))
	for _, r := range resources {
		if !allNamespaces && !r.Namespaced {
			continue
		}
		filtered = append(filtered, r)
	}
	return filtered
}

func filterSystemNamespaces(resources []types.Resource) []types.Resource {
	filtered := []types.Resource{}
	for _, resource := range resources {
		if resource.Namespace == "kube-system" || resource.Namespace == "kube-public" || resource.Namespace == "kube-node-lease" {
			continue
		}
		filtered = append(filtered, resource)
	}
	return filtered
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
