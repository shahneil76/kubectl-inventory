package output

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/shahneil76/kubectl-inventory/pkg/types"
)

func RenderTree(w io.Writer, inv types.Inventory) error {
	header := "Namespace: " + inv.Namespace
	if inv.AllNamespaces {
		header = "Namespace: all"
	}
	fmt.Fprintln(w, header)

	children := map[string][]types.Resource{}
	roots := []types.Resource{}
	byUID := map[string]types.Resource{}
	for _, resource := range inv.Resources {
		byUID[string(resource.UID)] = resource
	}
	for _, resource := range inv.Resources {
		parent := ""
		for _, owner := range resource.OwnerRefs {
			if _, ok := byUID[string(owner.UID)]; ok {
				parent = string(owner.UID)
				break
			}
		}
		if parent == "" {
			roots = append(roots, resource)
		} else {
			children[parent] = append(children[parent], resource)
		}
	}
	sortResources(roots)
	for i, root := range roots {
		last := i == len(roots)-1
		printNode(w, root, children, "", last)
	}
	return nil
}

func printNode(w io.Writer, resource types.Resource, children map[string][]types.Resource, prefix string, last bool) {
	connector := "├── "
	nextPrefix := prefix + "│   "
	if last {
		connector = "└── "
		nextPrefix = prefix + "    "
	}
	label := resource.Kind + "/" + resource.Name
	if resource.Namespace != "" {
		label += " [" + resource.Namespace + "]"
	}
	if resource.IsOrphaned {
		label = "[SUSPICIOUS] " + label
	}
	if resource.IsStuck {
		label += " [STUCK]"
	}
	if resource.Age > 0 && resource.IsOrphaned {
		label += " (" + strings.TrimSuffix(resource.Age.Round(24*60*60*1_000_000_000).String(), "0s") + " old)"
	}
	fmt.Fprintln(w, prefix+connector+label)

	kids := children[string(resource.UID)]
	sortResources(kids)
	for i, child := range kids {
		printNode(w, child, children, nextPrefix, i == len(kids)-1)
	}
}

func sortResources(resources []types.Resource) {
	sort.Slice(resources, func(i, j int) bool {
		left := resources[i].Kind + "/" + resources[i].Namespace + "/" + resources[i].Name
		right := resources[j].Kind + "/" + resources[j].Namespace + "/" + resources[j].Name
		return left < right
	})
}
