package analyzer

import "github.com/shahneil76/kubectl-inventory/pkg/types"

type Coverage struct {
	Total     int            `json:"total"`
	Managed   int            `json:"managed"`
	Unmanaged int            `json:"unmanaged"`
	ByTool    map[string]int `json:"byTool"`
}

func GitOpsCoverage(resources []types.Resource) Coverage {
	coverage := Coverage{ByTool: map[string]int{}}
	coverage.Total = len(resources)
	for _, resource := range resources {
		if resource.GitOpsTool == "" {
			coverage.Unmanaged++
			continue
		}
		coverage.Managed++
		coverage.ByTool[resource.GitOpsTool]++
	}
	return coverage
}

func Percent(part, total int) int {
	if total == 0 {
		return 0
	}
	return int(float64(part) / float64(total) * 100)
}
