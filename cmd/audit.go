package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/shahneil76/kubectl-inventory/pkg/audit"
	"github.com/shahneil76/kubectl-inventory/pkg/audit/report"
	"github.com/spf13/cobra"
)

var auditOutputFile string

var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Run inventory-native cluster posture audit",
	Long: `Posture audit combines best-practice checks with inventory-native signals
(dangling owners, stuck finalizers, GitOps drift) into a ranked action queue.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		inv, err := scanInventory(cmd.Context(), opts.Namespace, opts.AllNamespaces)
		if err != nil {
			return err
		}

		ns := opts.Namespace
		if opts.AllNamespaces {
			ns = "*"
		}
		posture := audit.RunPostureFromInventory(inv.Resources, ns)
		cfg := audit.DefaultConfig()
		filtered := audit.ApplySettings(posture.ToScanResults(), cfg.IgnoredNamespaces, cfg.DisabledChecks)
		posture = audit.BuildPostureReport(inv.Resources, filtered, ns)

		if opts.Output == "pdf" {
			out := auditOutputFile
			if out == "" {
				out = fmt.Sprintf("kubectl-inventory-posture-%s.pdf", time.Now().Format("20060102-150405"))
			}
			meta := report.Meta{Context: inv.Context, Namespace: ns, Generated: time.Now()}
			data, err := report.GeneratePDF(posture, meta)
			if err != nil {
				return err
			}
			return os.WriteFile(out, data, 0o644)
		}

		if opts.Output == "json" {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(posture)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "%s\n\n", posture.Posture.Headline)
		fmt.Fprintln(cmd.OutOrStdout(), posture.Posture.Description)
		fmt.Fprintf(cmd.OutOrStdout(), "\nMust-fix=%d  Advisory=%d  Inventory-native=%d  Compound=%d\n\n",
			posture.Summary.Danger, posture.Summary.Warning, posture.InventoryFindings, posture.CompoundFindings)

		if len(posture.ActionQueue) > 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "Priority action queue:")
			limit := len(posture.ActionQueue)
			if limit > 10 {
				limit = 10
			}
			for i := 0; i < limit; i++ {
				item := posture.ActionQueue[i]
				fmt.Fprintf(cmd.OutOrStdout(), "  #%d [%s] %s/%s — %s (%s)\n",
					item.Rank, strings.ToUpper(item.Severity), item.Kind, item.Name, item.Title, item.Reason)
			}
			fmt.Fprintln(cmd.OutOrStdout())
		}

		if len(posture.Groups) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "All checks passing ✓")
			return nil
		}

		for _, g := range posture.Groups {
			fmt.Fprintf(cmd.OutOrStdout(), "%s/%s [%s]  must-fix=%d advisory=%d\n",
				g.Kind, g.Name, g.Namespace, g.Danger, g.Warning)
			for _, f := range g.Findings {
				meta := posture.Checks[f.CheckID]
				title := f.CheckID
				if meta.Title != "" {
					title = meta.Title
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  [%s] %s — %s\n", strings.ToUpper(f.Severity), title, f.Message)
			}
			fmt.Fprintln(cmd.OutOrStdout())
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(auditCmd)
	auditCmd.Flags().StringVar(&auditOutputFile, "file", "", "Output path for PDF export (with --output pdf)")
}
