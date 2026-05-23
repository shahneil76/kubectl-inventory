package report

import (
	"testing"
	"time"

	"github.com/shahneil76/kubectl-inventory/pkg/audit"
)

func TestGeneratePDF(t *testing.T) {
	report := &audit.PostureReport{
		Summary: audit.ScanSummary{
			Danger:  2,
			Warning: 5,
			Categories: map[string]audit.CategorySummary{
				audit.CategorySecurity:    {Danger: 1, Warning: 2},
				audit.CategoryReliability: {Danger: 1, Warning: 2},
				audit.CategoryEfficiency:  {Warning: 1},
			},
		},
		Posture: audit.PostureSnapshot{
			Score:       72,
			Grade:       "C",
			Headline:    "Posture Index 72 — Grade C",
			Description: "test cluster posture",
		},
		ActionQueue: []audit.ActionItem{{
			Rank: 1, Impact: 80, Kind: "Deployment", Name: "web", Namespace: "app",
			Title: "Run as non-root", Severity: audit.SeverityDanger, Lens: audit.LensHardening,
			Reason: "Compound risk",
		}},
		NamespacePostures: []audit.NamespacePosture{{
			Namespace: "app", Score: 65, Grade: "D", MustFix: 2, Advisory: 3,
			InventorySignals: map[string]int{"DANG": 1},
		}},
		EnrichedFindings: []audit.EnrichedFinding{{
			Finding: audit.Finding{
				Kind: "Deployment", Name: "web", Namespace: "app",
				CheckID: "runAsRoot", Severity: audit.SeverityDanger,
				Message: "container runs as root",
			},
			Lens: audit.LensHardening, ImpactScore: 70,
		}},
		Checks: audit.CheckRegistry,
	}

	data, err := GeneratePDF(report, Meta{Context: "test", Namespace: "*", Generated: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 1000 {
		t.Fatalf("pdf too small: %d bytes", len(data))
	}
	if data[0] != '%' || data[1] != 'P' || data[2] != 'D' || data[3] != 'F' {
		t.Fatal("invalid pdf header")
	}
}
