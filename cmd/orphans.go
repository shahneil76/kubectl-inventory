package cmd

import (
	"fmt"

	"github.com/shahneil76/kubectl-inventory/pkg/analyzer"
	"github.com/spf13/cobra"
)

var orphansCmd = &cobra.Command{
	Use:   "orphans",
	Short: "Show suspicious orphan candidates with reasons",
	RunE: func(cmd *cobra.Command, args []string) error {
		inv, err := scanInventory(cmd.Context(), opts.Namespace, opts.AllNamespaces)
		if err != nil {
			return err
		}
		orphans := analyzer.MarkOrphans(inv.Resources, opts.Age)
		inv.Resources = orphans
		if opts.Output == "table" || opts.Output == "" || opts.Output == "wide" {
			if len(inv.Resources) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No suspicious orphan candidates found ✓")
				return nil
			}
			for _, resource := range inv.Resources {
				label := "SUSPICIOUS"
				if resource.OrphanStatus == "dangling" {
					label = "DANGLING "
				}
				age := resource.Age.Round(24 * 60 * 60 * 1_000_000_000).String()
				fmt.Fprintf(cmd.OutOrStdout(), "%s %s/%s\t[%s]\t%s\n", label, resource.Kind, resource.Name, resource.Namespace, age)
				for _, reason := range resource.OrphanReasons {
					fmt.Fprintf(cmd.OutOrStdout(), "  reason: %s\n", reason)
				}
			}
			return nil
		}
		return renderInventory(cmd, inv)
	},
}

func init() {
	rootCmd.AddCommand(orphansCmd)
}
