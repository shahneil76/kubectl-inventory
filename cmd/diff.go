package cmd

import (
	"fmt"

	"github.com/shahneil76/kubectl-inventory/pkg/output"
	"github.com/spf13/cobra"
)

var diffCmd = &cobra.Command{
	Use:   "diff -n <from> -n <to>",
	Short: "Diff two namespaces",
	RunE: func(cmd *cobra.Command, args []string) error {
		namespaces, err := cmd.Flags().GetStringArray("namespace")
		if err != nil {
			return err
		}
		if len(namespaces) != 2 {
			return fmt.Errorf("diff requires exactly two -n/--namespace values")
		}
		from, err := scanInventory(cmd.Context(), namespaces[0], false)
		if err != nil {
			return err
		}
		to, err := scanInventory(cmd.Context(), namespaces[1], false)
		if err != nil {
			return err
		}
		report := output.DiffInventories(from, to)
		return output.RenderDiff(cmd.OutOrStdout(), report)
	},
}

func init() {
	diffCmd.Flags().StringArrayP("namespace", "n", nil, "Namespace to compare; pass exactly twice")
	rootCmd.AddCommand(diffCmd)
}
