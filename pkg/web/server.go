package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/shahneil76/kubectl-inventory/pkg/types"
)

// Server holds the HTTP server and inventory state.
type Server struct {
	inv    *types.Inventory
	port   int
	server *http.Server
}

// New creates a new web server with the given inventory and port.
func New(inv *types.Inventory, port int) *Server {
	s := &Server{inv: inv, port: port}
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("/api/inventory", s.handleInventory)
	mux.HandleFunc("/api/radar", s.handleRadar)
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/alerts", s.handleAlerts)
	mux.HandleFunc("/api/namespaces", s.handleNamespaces)
	mux.HandleFunc("/api/contexts", s.handleContexts)
	mux.HandleFunc("/api/resources", s.handleResources)
	mux.HandleFunc("/api/canvas", s.handleCanvas)

	// Static UI
	mux.Handle("/", http.FileServer(staticFiles))

	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      corsMiddleware(mux),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	return s
}

// Start begins listening and returns the bound address.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.server.Addr)
	if err != nil {
		return fmt.Errorf("bind port %d: %w", s.port, err)
	}
	go func() { _ = s.server.Serve(ln) }()
	return nil
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

// URL returns the server URL.
func (s *Server) URL() string {
	return fmt.Sprintf("http://localhost:%d", s.port)
}

// ─── CORS middleware ──────────────────────────────────────────────────────────

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ─── API helpers ─────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// ─── API response types ───────────────────────────────────────────────────────

type RadarGroup struct {
	Group     string        `json:"group"`
	Resources []RadarSignal `json:"resources"`
}

type RadarSignal struct {
	Name    string         `json:"name"`
	Kind    string         `json:"kind"`
	Total   int            `json:"total"`
	Signals map[string]int `json:"signals"`
}

type HealthStat struct {
	Label string `json:"label"`
	Count int    `json:"count"`
	Class string `json:"class"`
}

type AlertItem struct {
	ID        string `json:"id"`
	Signal    string `json:"signal"`
	Severity  string `json:"severity"`
	Resource  string `json:"resource"`
	APIGroup  string `json:"apiGroup"`
	Namespace string `json:"namespace"`
	Issue     string `json:"issue"`
	Age       string `json:"age"`
}

type NamespaceItem struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type ResourceRow struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	Group     string `json:"group"`
	Age       string `json:"age"`
	Signal    string `json:"signal"`
	Reason    string `json:"reason"`
}

// ─── Handlers ────────────────────────────────────────────────────────────────

// handleInventory returns the raw inventory summary.
func (s *Server) handleInventory(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"context":       s.inv.Context,
		"namespace":     s.inv.Namespace,
		"allNamespaces": s.inv.AllNamespaces,
		"stats":         s.inv.Stats,
		"totalResources": len(s.inv.Resources),
	})
}

// handleRadar returns grouped resource signals for the Radar screen.
func (s *Server) handleRadar(w http.ResponseWriter, r *http.Request) {
	ns := r.URL.Query().Get("namespace")

	// Group resources by API group category
	groupMap := map[string]*RadarGroup{}
	kindGroupMap := map[string]string{} // kind -> display group

	categoryFor := func(group, kind string) string {
		switch {
		case group == "" || group == "apps" || group == "batch":
			return "Workloads"
		case group == "networking.k8s.io" || kind == "Service" || kind == "Ingress" || kind == "Endpoints" || kind == "EndpointSlice":
			return "Networking"
		case kind == "ConfigMap" || kind == "Secret" || kind == "ResourceQuota" || kind == "LimitRange":
			return "Configuration"
		case group == "rbac.authorization.k8s.io" || group == "policy":
			return "RBAC & Policy"
		case group == "storage.k8s.io":
			return "Storage"
		case strings.Contains(group, "."):
			return group // CRD groups shown as-is
		default:
			return "Core"
		}
	}

	for _, res := range s.inv.Resources {
		if ns != "" && ns != "*" && res.Namespace != ns {
			continue
		}

		cat := categoryFor(res.Group, res.Kind)
		if _, ok := groupMap[cat]; !ok {
			groupMap[cat] = &RadarGroup{Group: cat}
		}
		grp := groupMap[cat]

		// Find or create signal entry for this kind
		kindGroupMap[res.Kind] = cat
		idx := -1
		for i, sig := range grp.Resources {
			if sig.Kind == res.Kind {
				idx = i
				break
			}
		}
		if idx == -1 {
			grp.Resources = append(grp.Resources, RadarSignal{
				Name:    res.Resource,
				Kind:    res.Kind,
				Signals: map[string]int{},
			})
			idx = len(grp.Resources) - 1
		}

		grp.Resources[idx].Total++
		sig := signalFor(res)
		grp.Resources[idx].Signals[sig]++
	}

	_ = kindGroupMap

	// Convert to sorted slice
	groups := make([]RadarGroup, 0, len(groupMap))
	for _, g := range groupMap {
		groups = append(groups, *g)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Group < groups[j].Group })
	writeJSON(w, groups)
}

// handleHealth returns health summary stats.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	counts := map[string]int{
		"clean":      0,
		"owned":      0,
		"generated":  0,
		"referenced": 0,
		"suspicious": 0,
		"dangling":   0,
		"stuck":      0,
		"standalone": 0,
	}
	for _, res := range s.inv.Resources {
		st := res.OrphanStatus
		if st == "" {
			st = "standalone"
		}
		counts[st]++
	}

	stats := []HealthStat{
		{Label: "Clean", Count: counts["clean"], Class: "clean"},
		{Label: "Owned", Count: counts["owned"], Class: "owned"},
		{Label: "Generated", Count: counts["generated"], Class: "gen"},
		{Label: "Referenced", Count: counts["referenced"], Class: "ref"},
		{Label: "Suspicious", Count: counts["suspicious"], Class: "susp"},
		{Label: "Dangling", Count: counts["dangling"], Class: "dang"},
		{Label: "Stuck", Count: counts["stuck"], Class: "stuck"},
		{Label: "Standalone", Count: counts["standalone"], Class: "standalone"},
	}
	writeJSON(w, stats)
}

// handleAlerts returns DANG and STUCK signal resources.
func (s *Server) handleAlerts(w http.ResponseWriter, r *http.Request) {
	sigFilter := strings.ToLower(r.URL.Query().Get("signal"))

	var alerts []AlertItem
	for _, res := range s.inv.Resources {
		sig := signalFor(res)
		if sig != "DANG" && sig != "STUCK" {
			continue
		}
		if sigFilter != "" && strings.ToLower(sig) != sigFilter {
			continue
		}
		sev := "HIGH-2"
		if sig == "DANG" {
			sev = "CRIT-1"
		}
		issue := ""
		if len(res.OrphanReasons) > 0 {
			issue = strings.Join(res.OrphanReasons, "; ")
		} else if res.IsStuck {
			issue = fmt.Sprintf("Stuck finalizers: %s", strings.Join(res.Finalizers, ", "))
		} else {
			issue = "Dangling resource – all owners gone"
		}
		group := res.Group
		if group == "" {
			group = "core/v1"
		}
		alerts = append(alerts, AlertItem{
			ID:        string(res.UID),
			Signal:    sig,
			Severity:  sev,
			Resource:  fmt.Sprintf("%s/%s", strings.ToLower(res.Kind), res.Name),
			APIGroup:  group,
			Namespace: res.Namespace,
			Issue:     issue,
			Age:       formatAge(res.Age),
		})
	}

	// Sort: DANG first, then by age desc
	sort.Slice(alerts, func(i, j int) bool {
		if alerts[i].Signal != alerts[j].Signal {
			return alerts[i].Signal == "DANG"
		}
		return true
	})
	writeJSON(w, alerts)
}

// handleNamespaces returns all unique namespaces with resource counts.
func (s *Server) handleNamespaces(w http.ResponseWriter, r *http.Request) {
	nsMap := map[string]int{}
	for _, res := range s.inv.Resources {
		if res.Namespace != "" {
			nsMap[res.Namespace]++
		}
	}
	items := []NamespaceItem{}
	for ns, count := range nsMap {
		items = append(items, NamespaceItem{Name: ns, Count: count})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	writeJSON(w, items)
}

// handleContexts returns the current context info.
func (s *Server) handleContexts(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, []map[string]any{
		{
			"name":      s.inv.Context,
			"active":    true,
			"resources": len(s.inv.Resources),
			"lastScan":  "just now",
			"status":    "CONNECTED",
		},
	})
}

// handleResources returns filtered resource rows for drill-down.
func (s *Server) handleResources(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	group := r.URL.Query().Get("group")
	ns := r.URL.Query().Get("namespace")
	signal := strings.ToUpper(r.URL.Query().Get("signal"))

	var rows []ResourceRow
	for _, res := range s.inv.Resources {
		if kind != "" && !strings.EqualFold(res.Kind, kind) {
			continue
		}
		if group != "" && group != "*" && res.Group != group {
			continue
		}
		if ns != "" && ns != "*" && res.Namespace != ns {
			continue
		}
		sig := signalFor(res)
		if signal != "" && sig != signal {
			continue
		}
		reason := ""
		if len(res.OrphanReasons) > 0 {
			reason = strings.Join(res.OrphanReasons, "; ")
		}
		rows = append(rows, ResourceRow{
			Name:      res.Name,
			Namespace: res.Namespace,
			Kind:      res.Kind,
			Group:     res.Group,
			Age:       formatAge(res.Age),
			Signal:    sig,
			Reason:    reason,
		})
	}
	writeJSON(w, rows)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func signalFor(res types.Resource) string {
	switch res.OrphanStatus {
	case types.OrphanStatusDangling:
		return "DANG"
	case types.OrphanStatusSuspicious:
		return "SUSP"
	case types.OrphanStatusOwned:
		return "OWNED"
	case types.OrphanStatusGenerated:
		return "GEN"
	case types.OrphanStatusReferenced:
		return "REF"
	case types.OrphanStatusManaged:
		return "OWNED"
	}
	if res.IsStuck {
		return "STUCK"
	}
	if res.IsOwned {
		return "OWNED"
	}
	return "CLEAN"
}

func formatAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// ─── Canvas types & handler ───────────────────────────────────────────────────

type CanvasNode struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Kind      string            `json:"kind"`
	Namespace string            `json:"namespace"`
	Signal    string            `json:"signal"`
	Age       string            `json:"age"`
	Meta      map[string]string `json:"meta"`
}

type CanvasEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"` // owner | spec | gen | dang
}

type CanvasResponse struct {
	Nodes []CanvasNode `json:"nodes"`
	Edges []CanvasEdge `json:"edges"`
}

// handleCanvas returns real resource instances with owner-ref edges for the canvas view.
// Strategy: find root nodes that have children, BFS their subtrees first, then fill
// remaining slots with high-signal isolated nodes.
func (s *Server) handleCanvas(w http.ResponseWriter, r *http.Request) {
	ns := r.URL.Query().Get("namespace")

	// Build full UID map
	uidMap := map[string]types.Resource{}
	for _, res := range s.inv.Resources {
		uidMap[string(res.UID)] = res
	}

	// Filter by namespace
	var pool []types.Resource
	for _, res := range s.inv.Resources {
		if ns != "" && ns != "*" && res.Namespace != ns {
			continue
		}
		pool = append(pool, res)
	}

	// Build parent/child maps within the pool
	poolUIDs := map[string]bool{}
	for _, r := range pool {
		poolUIDs[string(r.UID)] = true
	}
	childUIDs := map[string]bool{} // UIDs that have at least one child in pool
	parentUID := map[string]string{} // child UID -> parent UID (first owner in pool)
	for _, res := range pool {
		for _, ref := range res.OwnerRefs {
			ownerUID := string(ref.UID)
			if poolUIDs[ownerUID] {
				childUIDs[ownerUID] = true
				if parentUID[string(res.UID)] == "" {
					parentUID[string(res.UID)] = ownerUID
				}
			}
		}
	}

	// Prefer "root" resources that have at least one child (real tree roots)
	var treeRoots, leaves, isolated []types.Resource
	for _, res := range pool {
		uid := string(res.UID)
		hasParent := parentUID[uid] != ""
		hasChild := childUIDs[uid]
		switch {
		case !hasParent && hasChild:
			treeRoots = append(treeRoots, res)
		case hasParent:
			leaves = append(leaves, res)
		default:
			isolated = append(isolated, res)
		}
	}

	// Sort tree roots by number of descendants (prefer richer trees)
	sort.SliceStable(treeRoots, func(i, j int) bool {
		return signalScore(signalFor(treeRoots[i])) > signalScore(signalFor(treeRoots[j]))
	})

	// Sort isolated by signal score descending
	sort.SliceStable(isolated, func(i, j int) bool {
		return signalScore(signalFor(isolated[i])) > signalScore(signalFor(isolated[j]))
	})

	// BFS from tree roots up to maxNodes
	const maxNodes = 40
	included := map[string]bool{}
	var selected []types.Resource

	// Index pool by UID for quick lookup
	poolByUID := map[string]types.Resource{}
	for _, r := range pool {
		poolByUID[string(r.UID)] = r
	}
	// Build childrenOf map
	childrenOf := map[string][]string{}
	for _, res := range pool {
		for _, ref := range res.OwnerRefs {
			ownerUID := string(ref.UID)
			if poolUIDs[ownerUID] {
				childrenOf[ownerUID] = append(childrenOf[ownerUID], string(res.UID))
			}
		}
	}

	var bfsQueue []string
	for _, root := range treeRoots {
		if len(selected) >= maxNodes {
			break
		}
		uid := string(root.UID)
		if included[uid] {
			continue
		}
		bfsQueue = append(bfsQueue, uid)
		for len(bfsQueue) > 0 && len(selected) < maxNodes {
			cur := bfsQueue[0]
			bfsQueue = bfsQueue[1:]
			if included[cur] {
				continue
			}
			included[cur] = true
			if res, ok := poolByUID[cur]; ok {
				selected = append(selected, res)
			}
			for _, childUID := range childrenOf[cur] {
				if !included[childUID] {
					bfsQueue = append(bfsQueue, childUID)
				}
			}
		}
	}

	// Fill remaining with leaves that have parents already selected
	for _, res := range leaves {
		if len(selected) >= maxNodes {
			break
		}
		uid := string(res.UID)
		if !included[uid] && included[parentUID[uid]] {
			included[uid] = true
			selected = append(selected, res)
		}
	}

	// Fill remaining with isolated high-signal resources
	for _, res := range isolated {
		if len(selected) >= maxNodes {
			break
		}
		uid := string(res.UID)
		if !included[uid] {
			included[uid] = true
			selected = append(selected, res)
		}
	}

	// Build nodes
	nodes := make([]CanvasNode, 0, len(selected))
	for _, res := range selected {
		api := res.Group
		if api == "" {
			api = "core/v1"
		} else {
			api = api + "/" + res.Version
		}
		nodes = append(nodes, CanvasNode{
			ID:        string(res.UID),
			Name:      res.Name,
			Kind:      res.Kind,
			Namespace: res.Namespace,
			Signal:    signalFor(res),
			Age:       formatAge(res.Age),
			Meta: map[string]string{
				"KIND": res.Kind,
				"API":  api,
				"NS":   res.Namespace,
			},
		})
	}

	// Build edges from owner references
	includedUIDs := included
	var edges []CanvasEdge
	for _, res := range selected {
		sig := signalFor(res)
		for _, ref := range res.OwnerRefs {
			ownerUID := string(ref.UID)
			if !includedUIDs[ownerUID] {
				continue
			}
			edgeType := "owner"
			if sig == "GEN" {
				edgeType = "gen"
			} else if sig == "DANG" {
				edgeType = "dang"
			}
			edges = append(edges, CanvasEdge{
				From: ownerUID,
				To:   string(res.UID),
				Type: edgeType,
			})
		}
	}

	writeJSON(w, CanvasResponse{Nodes: nodes, Edges: edges})
}


func signalScore(sig string) int {
	switch sig {
	case "DANG":
		return 5
	case "STUCK":
		return 4
	case "SUSP":
		return 3
	case "GEN":
		return 2
	case "REF":
		return 1
	default:
		return 0
	}
}
