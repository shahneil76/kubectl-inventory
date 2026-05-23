package audit

import (
	"fmt"
	"sort"
	"strings"

	"github.com/shahneil76/kubectl-inventory/pkg/types"
)

// Lens names used by kubectl-inventory (distinct from Radar's category labels).
const (
	LensHardening    = "hardening"
	LensResilience   = "resilience"
	LensRightsizing  = "rightsizing"
	LensInventoryDNA = "inventory" // inventory-native signals only we emit
)

// PostureReport wraps scan output with inventory-native posture intelligence.
// This is the primary API/CLI shape — not a raw Polaris-style dump.
type PostureReport struct {
	Summary            ScanSummary                `json:"summary"`
	Findings           []Finding                  `json:"findings"`
	Groups             []ResourceGroup            `json:"groups"`
	Checks             map[string]CheckMeta     `json:"checks"`
	Posture            PostureSnapshot            `json:"posture"`
	NamespacePostures  []NamespacePosture         `json:"namespacePostures"`
	ActionQueue        []ActionItem               `json:"actionQueue"`
	EnrichedFindings   []EnrichedFinding          `json:"enrichedFindings"`
	InventoryFindings  int                        `json:"inventoryFindings"`
	CompoundFindings   int                        `json:"compoundFindings"`
}

type PostureSnapshot struct {
	Score       int    `json:"score"`
	Grade       string `json:"grade"`
	Headline    string `json:"headline"`
	Description string `json:"description"`
}

type NamespacePosture struct {
	Namespace        string         `json:"namespace"`
	Score            int            `json:"score"`
	Grade            string         `json:"grade"`
	MustFix          int            `json:"mustFix"`
	Advisory         int            `json:"advisory"`
	InventorySignals map[string]int `json:"inventorySignals"`
}

type ActionItem struct {
	Rank        int    `json:"rank"`
	Impact      int    `json:"impact"`
	Kind        string `json:"kind"`
	Namespace   string `json:"namespace"`
	Name        string `json:"name"`
	CheckID     string `json:"checkID"`
	Title       string `json:"title"`
	Severity    string `json:"severity"`
	Lens        string `json:"lens"`
	Reason      string `json:"reason"`
	Compound    bool   `json:"compound"`
}

type EnrichedFinding struct {
	Finding
	Lens            string `json:"lens"`
	InventorySignal string `json:"inventorySignal,omitempty"`
	ImpactScore     int    `json:"impactScore"`
	Compound        bool   `json:"compound"`
}

var inventoryCheckRegistry = map[string]CheckMeta{
	"inventory:dangling": {
		ID:          "inventory:dangling",
		Title:       "Dangling owner reference",
		Description: "Resource claims owners that no longer exist in the cluster — a lifecycle integrity failure unique to deep inventory scans.",
		Remediation: "Verify the controller is healthy, restore the missing owner, or remove stale ownerReferences if the resource is truly orphaned.",
	},
	"inventory:stuck": {
		ID:          "inventory:stuck",
		Title:       "Stuck finalizer lock",
		Description: "Deletion is blocked by finalizers while the object remains in inventory — often a broken operator or webhook.",
		Remediation: "Identify the controller responsible for each finalizer and restore it before clearing finalizers manually.",
	},
	"inventory:suspicious": {
		ID:          "inventory:suspicious",
		Title:       "Suspicious orphan candidate",
		Description: "No owner chain and weak cross-references — may be abandoned configuration or drift from GitOps.",
		Remediation: "Confirm whether this resource is still required; delete or attach proper ownership labels and controllers.",
	},
	"inventory:unmanaged": {
		ID:          "inventory:unmanaged",
		Title:       "Unmanaged workload in GitOps namespace",
		Description: "Deployment-like workload lacks GitOps management signals while sibling resources in the namespace are managed.",
		Remediation: "Adopt the workload into Argo CD, Flux, or Helm — or move it to a namespace intended for ad-hoc resources.",
	},
}

// BuildPostureReport merges best-practice scan results with inventory-native signals.
func BuildPostureReport(resources []types.Resource, scan *ScanResults, namespace string) *PostureReport {
	if scan == nil {
		scan = &ScanResults{Summary: ScanSummary{Categories: map[string]CategorySummary{}}}
	}

	nsFilter := namespace != "" && namespace != "*"
	resourceIndex := indexInventoryResources(resources, nsFilter, namespace)
	inventoryFindings := synthesizeInventoryFindings(resources, nsFilter, namespace)

	allFindings := append(append([]Finding{}, scan.Findings...), inventoryFindings...)
	groups := GroupByResource(allFindings)

	checks := map[string]CheckMeta{}
	for k, v := range CheckRegistry {
		checks[k] = v
	}
	for k, v := range inventoryCheckRegistry {
		checks[k] = v
	}

	summary := resummary(allFindings)
	enriched := enrichFindings(allFindings, resourceIndex, checks)
	actionQueue := buildActionQueue(enriched, checks)
	nsPostures := buildNamespacePostures(enriched, resourceIndex)
	posture := clusterPosture(summary, enriched, len(resourceIndex))

	return &PostureReport{
		Summary:           summary,
		Findings:          allFindings,
		Groups:            groups,
		Checks:            checks,
		Posture:             posture,
		NamespacePostures: nsPostures,
		ActionQueue:       actionQueue,
		EnrichedFindings:  enriched,
		InventoryFindings: len(inventoryFindings),
		CompoundFindings:  countCompound(enriched),
	}
}

func indexInventoryResources(resources []types.Resource, nsFilter bool, namespace string) map[string]types.Resource {
	m := make(map[string]types.Resource)
	for _, res := range resources {
		if nsFilter && res.Namespace != namespace {
			continue
		}
		m[ResourceKey(res.Group, res.Kind, res.Namespace, res.Name)] = res
	}
	return m
}

func synthesizeInventoryFindings(resources []types.Resource, nsFilter bool, namespace string) []Finding {
	gitOpsNs := detectGitOpsNamespaces(resources)
	var out []Finding

	for _, res := range resources {
		if nsFilter && res.Namespace != namespace {
			continue
		}

		switch res.OrphanStatus {
		case types.OrphanStatusDangling:
			out = append(out, inventoryFinding(res, "inventory:dangling", SeverityDanger,
				"Dangling resource — all owner references are missing from inventory"))
		case types.OrphanStatusSuspicious:
			out = append(out, inventoryFinding(res, "inventory:suspicious", SeverityWarning,
				"Suspicious orphan candidate — weak ownership and reference graph"))
		}
		if res.IsStuck {
			out = append(out, inventoryFinding(res, "inventory:stuck", SeverityDanger,
				fmt.Sprintf("Stuck terminating — finalizers: %s", strings.Join(res.Finalizers, ", "))))
		}
		if isUnmanagedWorkload(res, gitOpsNs[res.Namespace]) {
			out = append(out, inventoryFinding(res, "inventory:unmanaged", SeverityWarning,
				"Workload appears unmanaged while namespace has GitOps-managed resources"))
		}
	}
	return out
}

func inventoryFinding(res types.Resource, checkID, severity, message string) Finding {
	return Finding{
		Kind:      res.Kind,
		Group:     res.Group,
		Namespace: res.Namespace,
		Name:      res.Name,
		CheckID:   checkID,
		Category:  CategoryReliability,
		Severity:  severity,
		Message:   message,
	}
}

func detectGitOpsNamespaces(resources []types.Resource) map[string]bool {
	counts := map[string]int{}
	managed := map[string]int{}
	for _, res := range resources {
		if res.Namespace == "" {
			continue
		}
		counts[res.Namespace]++
		if res.GitOpsTool != "" || res.OrphanStatus == types.OrphanStatusManaged {
			managed[res.Namespace]++
		}
	}
	out := map[string]bool{}
	for ns, total := range counts {
		if total >= 5 && managed[ns]*100/total >= 40 {
			out[ns] = true
		}
	}
	return out
}

func isUnmanagedWorkload(res types.Resource, gitOpsNamespace bool) bool {
	if !gitOpsNamespace || res.GitOpsTool != "" {
		return false
	}
	switch res.Kind {
	case "Deployment", "StatefulSet", "DaemonSet":
		return true
	default:
		return false
	}
}

func categoryToLens(category string) string {
	switch category {
	case CategorySecurity:
		return LensHardening
	case CategoryReliability:
		return LensResilience
	case CategoryEfficiency:
		return LensRightsizing
	default:
		return LensResilience
	}
}

func inventorySignal(res types.Resource) string {
	if res.IsStuck {
		return "STUCK"
	}
	switch res.OrphanStatus {
	case types.OrphanStatusDangling:
		return "DANG"
	case types.OrphanStatusSuspicious:
		return "SUSP"
	case types.OrphanStatusManaged, types.OrphanStatusOwned:
		return "OWNED"
	default:
		return "CLEAN"
	}
}

func enrichFindings(findings []Finding, resourceIndex map[string]types.Resource, checks map[string]CheckMeta) []EnrichedFinding {
	out := make([]EnrichedFinding, 0, len(findings))
	for _, f := range findings {
		key := ResourceKey(f.Group, f.Kind, f.Namespace, f.Name)
		res, ok := resourceIndex[key]
		signal := ""
		if ok {
			signal = inventorySignal(res)
		}
		lens := categoryToLens(f.Category)
		if strings.HasPrefix(f.CheckID, "inventory:") {
			lens = LensInventoryDNA
		}
		impact := impactScore(f, signal)
		compound := signal == "DANG" || signal == "STUCK" || (signal == "SUSP" && f.Severity == SeverityDanger)
		_ = checks
		out = append(out, EnrichedFinding{
			Finding:         f,
			Lens:            lens,
			InventorySignal: signal,
			ImpactScore:     impact,
			Compound:        compound,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ImpactScore != out[j].ImpactScore {
			return out[i].ImpactScore > out[j].ImpactScore
		}
		return out[i].CheckID < out[j].CheckID
	})
	return out
}

func impactScore(f Finding, signal string) int {
	score := 10
	if f.Severity == SeverityDanger {
		score += 40
	} else {
		score += 15
	}
	switch f.Category {
	case CategorySecurity:
		score += 20
	case CategoryReliability:
		score += 12
	}
	switch signal {
	case "STUCK":
		score += 25
	case "DANG":
		score += 20
	case "SUSP":
		score += 8
	}
	if strings.HasPrefix(f.CheckID, "inventory:") {
		score += 10
	}
	return score
}

func buildActionQueue(enriched []EnrichedFinding, checks map[string]CheckMeta) []ActionItem {
	const maxItems = 20
	queue := make([]ActionItem, 0, maxItems)
	seen := map[string]bool{}

	for _, ef := range enriched {
		key := ResourceKey(ef.Group, ef.Kind, ef.Namespace, ef.Name) + "|" + ef.CheckID
		if seen[key] {
			continue
		}
		seen[key] = true
		meta := checks[ef.CheckID]
		title := meta.Title
		if title == "" {
			title = ef.CheckID
		}
		reason := "Best-practice violation"
		if ef.Compound {
			reason = "Compound risk — audit issue plus inventory integrity signal"
		} else if strings.HasPrefix(ef.CheckID, "inventory:") {
			reason = "Inventory-native lifecycle signal"
		}
		queue = append(queue, ActionItem{
			Impact:     ef.ImpactScore,
			Kind:       ef.Kind,
			Namespace:  ef.Namespace,
			Name:       ef.Name,
			CheckID:    ef.CheckID,
			Title:      title,
			Severity:   ef.Severity,
			Lens:       ef.Lens,
			Reason:     reason,
			Compound:   ef.Compound,
		})
		if len(queue) >= maxItems {
			break
		}
	}

	for i := range queue {
		queue[i].Rank = i + 1
	}
	return queue
}

func buildNamespacePostures(enriched []EnrichedFinding, resourceIndex map[string]types.Resource) []NamespacePosture {
	type acc struct {
		mustFix, advisory int
		signals           map[string]int
	}
	buckets := map[string]*acc{}

	for _, ef := range enriched {
		ns := ef.Namespace
		if ns == "" {
			ns = "(cluster-scoped)"
		}
		if buckets[ns] == nil {
			buckets[ns] = &acc{signals: map[string]int{}}
		}
		if ef.Severity == SeverityDanger {
			buckets[ns].mustFix++
		} else {
			buckets[ns].advisory++
		}
	}

	for _, res := range resourceIndex {
		ns := res.Namespace
		if ns == "" {
			ns = "(cluster-scoped)"
		}
		if buckets[ns] == nil {
			buckets[ns] = &acc{signals: map[string]int{}}
		}
		sig := inventorySignal(res)
		if sig != "CLEAN" && sig != "OWNED" {
			buckets[ns].signals[sig]++
		}
	}

	out := make([]NamespacePosture, 0, len(buckets))
	for ns, a := range buckets {
		score := namespaceScore(a.mustFix, a.advisory, a.signals)
		out = append(out, NamespacePosture{
			Namespace:        ns,
			Score:            score,
			Grade:            scoreToGrade(score),
			MustFix:          a.mustFix,
			Advisory:         a.advisory,
			InventorySignals: a.signals,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score < out[j].Score
		}
		return out[i].Namespace < out[j].Namespace
	})
	return out
}

func resummary(findings []Finding) ScanSummary {
	categories := map[string]CategorySummary{}
	for _, cat := range []string{CategorySecurity, CategoryReliability, CategoryEfficiency} {
		categories[cat] = CategorySummary{}
	}
	var danger, warning int
	for _, f := range findings {
		cs := categories[f.Category]
		if f.Severity == SeverityDanger {
			danger++
			cs.Danger++
		} else {
			warning++
			cs.Warning++
		}
		categories[f.Category] = cs
	}
	return ScanSummary{Danger: danger, Warning: warning, Categories: categories}
}

func clusterPosture(summary ScanSummary, enriched []EnrichedFinding, resourceCount int) PostureSnapshot {
	if resourceCount <= 0 {
		resourceCount = 1
	}
	density := float64(summary.Danger*3+summary.Warning) / float64(resourceCount) * 100
	score := int(100 - density)
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	compound := countCompound(enriched)
	grade := scoreToGrade(score)
	headline := fmt.Sprintf("Posture Index %d — Grade %s", score, grade)
	desc := fmt.Sprintf("%d must-fix, %d advisory across %d inventoried resources", summary.Danger, summary.Warning, resourceCount)
	if compound > 0 {
		desc += fmt.Sprintf("; %d compound inventory+audit risks prioritized", compound)
	}
	return PostureSnapshot{Score: score, Grade: grade, Headline: headline, Description: desc}
}

func namespaceScore(mustFix, advisory int, signals map[string]int) int {
	penalty := mustFix*4 + advisory + signals["STUCK"]*5 + signals["DANG"]*4 + signals["SUSP"]*2
	score := 100 - penalty
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}

func scoreToGrade(score int) string {
	switch {
	case score >= 90:
		return "A"
	case score >= 75:
		return "B"
	case score >= 60:
		return "C"
	case score >= 40:
		return "D"
	default:
		return "F"
	}
}

func countCompound(enriched []EnrichedFinding) int {
	n := 0
	for _, ef := range enriched {
		if ef.Compound {
			n++
		}
	}
	return n
}

// ToScanResults returns the legacy scan shape for backwards compatibility.
func (p *PostureReport) ToScanResults() *ScanResults {
	if p == nil {
		return nil
	}
	return &ScanResults{
		Summary:  p.Summary,
		Findings: p.Findings,
		Groups:   p.Groups,
		Checks:   p.Checks,
	}
}
