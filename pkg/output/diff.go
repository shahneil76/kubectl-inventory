package output

import (
	"fmt"
	"io"
	"sort"

	"github.com/shahneil76/kubectl-inventory/pkg/types"
)

type DiffReport struct {
	FromNamespace string
	ToNamespace   string
	OnlyFrom      []string
	OnlyTo        []string
	Common        int
}

func DiffInventories(from, to types.Inventory) DiffReport {
	fromSet := map[string]types.Resource{}
	toSet := map[string]types.Resource{}
	for _, resource := range from.Resources {
		fromSet[resourceKey(resource)] = resource
	}
	for _, resource := range to.Resources {
		toSet[resourceKey(resource)] = resource
	}

	report := DiffReport{FromNamespace: from.Namespace, ToNamespace: to.Namespace}
	for key := range fromSet {
		if _, ok := toSet[key]; !ok {
			report.OnlyFrom = append(report.OnlyFrom, key)
		} else {
			report.Common++
		}
	}
	for key := range toSet {
		if _, ok := fromSet[key]; !ok {
			report.OnlyTo = append(report.OnlyTo, key)
		}
	}
	sort.Strings(report.OnlyFrom)
	sort.Strings(report.OnlyTo)
	return report
}

func RenderDiff(w io.Writer, report DiffReport) error {
	fmt.Fprintf(w, "Resource Diff: %s → %s\n", report.FromNamespace, report.ToNamespace)
	fmt.Fprintln(w, "────────────────────────────────────────────────")
	fmt.Fprintln(w, "✅  Only in "+report.FromNamespace+":")
	if len(report.OnlyFrom) == 0 {
		fmt.Fprintln(w, "    None")
	} else {
		for _, key := range report.OnlyFrom {
			fmt.Fprintln(w, "    "+key)
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "❌  Only in "+report.ToNamespace+":")
	if len(report.OnlyTo) == 0 {
		fmt.Fprintln(w, "    None")
	} else {
		for _, key := range report.OnlyTo {
			fmt.Fprintln(w, "    "+key)
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "✅  Common resources: %d\n", report.Common)
	return nil
}

func resourceKey(resource types.Resource) string {
	group := resource.Group
	if group != "" {
		group = "." + group
	}
	return fmt.Sprintf("%s%s/%s", resource.Kind, group, resource.Name)
}
