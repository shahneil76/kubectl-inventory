package analyzer

import "github.com/shahneil76/kubectl-inventory/pkg/types"

type NetworkReport struct {
	NetworkPolicies int      `json:"networkPolicies"`
	Ingresses       int      `json:"ingresses"`
	DeadIngresses   []string `json:"deadIngresses,omitempty"`
}

func AnalyzeNetwork(resources []types.Resource) NetworkReport {
	report := NetworkReport{}
	services := map[string]bool{}
	for _, resource := range resources {
		if resource.Kind == "Service" {
			services[resource.Namespace+"/"+resource.Name] = true
		}
		if resource.Kind == "NetworkPolicy" {
			report.NetworkPolicies++
		}
		if resource.Kind == "Ingress" {
			report.Ingresses++
		}
	}
	return report
}
