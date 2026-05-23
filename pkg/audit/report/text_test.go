package report

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/shahneil76/kubectl-inventory/pkg/audit"
)

func TestPDFText_replacesUnicodePunctuation(t *testing.T) {
	in := "Pod/test [DANGER] — Image tag · latest"
	out := pdfText(in)
	if strings.Contains(out, "\u2014") || strings.Contains(out, "\u00b7") {
		t.Fatalf("unicode punctuation not replaced: %q", out)
	}
	if !strings.Contains(out, " - ") {
		t.Fatalf("expected ASCII dash separator: %q", out)
	}
}

func TestGeneratePDF_noMojibake(t *testing.T) {
	report := &audit.PostureReport{
		Summary: audit.ScanSummary{
			Danger:  1,
			Warning: 1,
			Categories: map[string]audit.CategorySummary{
				audit.CategorySecurity: {Danger: 1, Warning: 1},
			},
		},
		Posture: audit.PostureSnapshot{
			Score:       55,
			Grade:       "D",
			Headline:    "Posture Index 55 — Grade D",
			Description: "Cluster needs work — dangling owners and privilege escalation.",
		},
		ActionQueue: []audit.ActionItem{{
			Rank: 1, Kind: "Pod", Name: "test", Namespace: "app",
			Title: "Image tag latest", Severity: audit.SeverityDanger,
			Reason: "Container \"web\" — uses :latest",
		}},
		EnrichedFindings: []audit.EnrichedFinding{{
			Finding: audit.Finding{
				Kind: "Pod", Name: "test", Namespace: "app",
				CheckID: "latestTag", Severity: audit.SeverityDanger,
				Message: "Container \"web\" uses image tag \":latest\" or missing tag",
			},
			Lens: audit.LensHardening,
		}},
		Checks: map[string]audit.CheckMeta{
			"latestTag": {
				ID:          "latestTag",
				Title:       "Image tag latest or missing",
				Remediation: "Pin images to a specific version tag — avoid :latest in production.",
			},
		},
	}

	data, err := GeneratePDF(report, Meta{Context: "test", Namespace: "*", Generated: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("â€")) || bytes.Contains(data, []byte("Â·")) {
		t.Fatalf("pdf contains mojibake: sample %q", string(data[:min(500, len(data))]))
	}
}
