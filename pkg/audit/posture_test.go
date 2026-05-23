package audit

import (
	"testing"

	"github.com/shahneil76/kubectl-inventory/pkg/types"
)

func TestBuildPostureReport_InventorySignals(t *testing.T) {
	resources := []types.Resource{
		{
			Kind: "Secret", Name: "orphan", Namespace: "app",
			OrphanStatus: types.OrphanStatusDangling,
		},
		{
			Kind: "Pod", Name: "stuck", Namespace: "app",
			IsStuck: true, Finalizers: []string{"kubernetes"},
		},
	}
	scan := &ScanResults{
		Summary: ScanSummary{Categories: map[string]CategorySummary{
			CategorySecurity: {}, CategoryReliability: {}, CategoryEfficiency: {},
		}},
		Checks: CheckRegistry,
	}

	report := BuildPostureReport(resources, scan, "*")
	if report.InventoryFindings < 2 {
		t.Fatalf("expected inventory-native findings, got %d", report.InventoryFindings)
	}
	if report.Posture.Score == 100 {
		t.Fatal("expected posture score to drop with inventory signals")
	}
	if len(report.ActionQueue) == 0 {
		t.Fatal("expected action queue entries")
	}
}

func TestBuildPostureReport_CompoundRisk(t *testing.T) {
	resources := []types.Resource{
		{
			Kind: "Deployment", Name: "web", Namespace: "prod",
			OrphanStatus: types.OrphanStatusDangling,
		},
	}
	scan := &ScanResults{
		Findings: []Finding{{
			Kind: "Deployment", Name: "web", Namespace: "prod",
			CheckID: "runAsRoot", Category: CategorySecurity, Severity: SeverityDanger,
			Message: "runs as root",
		}},
		Summary: ScanSummary{
			Danger: 1,
			Categories: map[string]CategorySummary{
				CategorySecurity:    {Danger: 1},
				CategoryReliability: {},
				CategoryEfficiency:  {},
			},
		},
		Checks: CheckRegistry,
	}

	report := BuildPostureReport(resources, scan, "*")
	if report.CompoundFindings == 0 {
		t.Fatal("expected compound findings when dangling + danger audit overlap")
	}
}
