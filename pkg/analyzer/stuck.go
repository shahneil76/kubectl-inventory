package analyzer

import "github.com/shahneil76/kubectl-inventory/pkg/types"

func StuckResources(resources []types.Resource) []types.Resource {
	stuck := []types.Resource{}
	for _, resource := range resources {
		if resource.IsStuck {
			stuck = append(stuck, resource)
		}
	}
	return stuck
}
