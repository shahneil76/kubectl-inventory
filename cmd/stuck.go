package cmd

import (
	"fmt"

	"github.com/shahneil76/kubectl-inventory/pkg/analyzer"
	"github.com/spf13/cobra"
)

var stuckCmd = &cobra.Command{
	Use:   "stuck",
	Short: "Show resources blocked by finalizers",
	RunE: func(cmd *cobra.Command, args []string) error {
		inv, err := scanInventory(cmd.Context(), opts.Namespace, opts.AllNamespaces)
		if err != nil {
			return err
		}
		inv.Resources = analyzer.StuckResources(inv.Resources)
		if opts.Output == "table" || opts.Output == "" || opts.Output == "wide" {
			if len(inv.Resources) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No stuck resources found ✓")
				return nil
			}
			for _, resource := range inv.Resources {
				fmt.Fprintf(cmd.OutOrStdout(), "%s/%s\t%s\t%v\n", resource.Kind, resource.Name, resource.Namespace, resource.Finalizers)
			}
			return nil
		}
		return renderInventory(cmd, inv)
	},
}

func init() {
	rootCmd.AddCommand(stuckCmd)
}
