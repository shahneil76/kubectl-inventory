package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/shahneil76/kubectl-inventory/pkg/audit"
	"github.com/shahneil76/kubectl-inventory/pkg/audit/report"
)

var (
	auditCache struct {
		mu        sync.RWMutex
		results   *audit.PostureReport
		nsKey     string
		expiresAt time.Time
	}
	auditCacheTTL = 5 * time.Second
)

func (s *Server) auditConfig() audit.Config {
	s.auditMu.RLock()
	defer s.auditMu.RUnlock()
	if s.auditSettings != nil {
		return *s.auditSettings
	}
	cfg := audit.DefaultConfig()
	return cfg
}

func (s *Server) setAuditConfig(cfg audit.Config) {
	s.auditMu.Lock()
	defer s.auditMu.Unlock()
	copy := cfg
	s.auditSettings = &copy
}

func (s *Server) getCachedAudit(namespace string) *audit.PostureReport {
	nsKey := namespace
	if nsKey == "" {
		nsKey = "*"
	}

	auditCache.mu.RLock()
	if auditCache.results != nil && auditCache.nsKey == nsKey && time.Now().Before(auditCache.expiresAt) {
		r := auditCache.results
		auditCache.mu.RUnlock()
		return r
	}
	auditCache.mu.RUnlock()

	results := audit.RunPostureFromInventory(s.inv.Resources, nsKey)
	cfg := s.auditConfig()
	filtered := audit.ApplySettings(results.ToScanResults(), cfg.IgnoredNamespaces, cfg.DisabledChecks)
	posture := audit.BuildPostureReport(s.inv.Resources, filtered, nsKey)

	auditCache.mu.Lock()
	auditCache.results = posture
	auditCache.nsKey = nsKey
	auditCache.expiresAt = time.Now().Add(auditCacheTTL)
	auditCache.mu.Unlock()

	return results
}

func (s *Server) filteredAudit(namespace string) *audit.PostureReport {
	return s.getCachedAudit(namespace)
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	ns := r.URL.Query().Get("namespace")
	if ns == "" {
		ns = "*"
	}
	writeJSON(w, s.filteredAudit(ns))
}

func (s *Server) handleAuditResource(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	namespace := r.URL.Query().Get("namespace")
	name := r.URL.Query().Get("name")
	ns := r.URL.Query().Get("namespaceFilter")
	if ns == "" {
		ns = "*"
	}

	results := s.filteredAudit(ns)
	index := audit.IndexByResource(results.Findings)
	key := audit.ResourceKey("", kind, namespace, name)
	findings := index[key]
	if findings == nil {
		findings = []audit.Finding{}
	}
	writeJSON(w, findings)
}

func (s *Server) handleGetAuditSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.auditConfig())
}

func (s *Server) handleAuditSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGetAuditSettings(w, r)
	case http.MethodPut:
		s.handlePutAuditSettings(w, r)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (s *Server) handlePutAuditSettings(w http.ResponseWriter, r *http.Request) {
	var cfg audit.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	s.setAuditConfig(cfg)
	auditCache.mu.Lock()
	auditCache.results = nil
	auditCache.mu.Unlock()
	writeJSON(w, cfg)
}

func (s *Server) handleAuditExport(w http.ResponseWriter, r *http.Request) {
	ns := r.URL.Query().Get("namespace")
	if ns == "" {
		ns = "*"
	}
	posture := s.filteredAudit(ns)
	meta := report.Meta{
		Context:   s.inv.Context,
		Namespace: ns,
		Generated: time.Now(),
	}
	pdfBytes, err := report.GeneratePDF(posture, meta)
	if err != nil {
		http.Error(w, `{"error":"failed to generate PDF"}`, http.StatusInternalServerError)
		return
	}
	filename := fmt.Sprintf("kubectl-inventory-posture-%s.pdf", time.Now().Format("20060102-150405"))
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	_, _ = w.Write(pdfBytes)
}
