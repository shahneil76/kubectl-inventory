package analyzer

import (
	"fmt"

	"github.com/shahneil76/kubectl-inventory/pkg/types"
)

type StorageReport struct {
	PVCs         int      `json:"pvcs"`
	Unmounted    []string `json:"unmounted,omitempty"`
	TotalStorage string   `json:"totalStorage,omitempty"`
}

func AnalyzeStorage(resources []types.Resource) StorageReport {
	report := StorageReport{}
	mountedPVCs := map[string]bool{}
	pvcs := map[string]types.Resource{}

	for _, resource := range resources {
		if resource.Kind == "Pod" && resource.Raw != nil {
			volumes, ok, _ := unstructuredSlice(resource.Raw.Object, "spec", "volumes")
			if ok {
				for _, volume := range volumes {
					claim, ok, _ := unstructuredMap(volume, "persistentVolumeClaim")
					if !ok {
						continue
					}
					claimName, _ := claim["claimName"].(string)
					if claimName != "" {
						mountedPVCs[resource.Namespace+"/"+claimName] = true
					}
				}
			}
		}
		if resource.Kind == "PersistentVolumeClaim" {
			report.PVCs++
			pvcs[resource.Namespace+"/"+resource.Name] = resource
		}
	}

	for key := range pvcs {
		if !mountedPVCs[key] {
			report.Unmounted = append(report.Unmounted, key)
		}
	}
	report.TotalStorage = fmt.Sprintf("%d PVCs", report.PVCs)
	return report
}

func unstructuredSlice(obj map[string]interface{}, fields ...string) ([]interface{}, bool, error) {
	current := interface{}(obj)
	for _, field := range fields {
		m, ok := current.(map[string]interface{})
		if !ok {
			return nil, false, nil
		}
		current, ok = m[field]
		if !ok {
			return nil, false, nil
		}
	}
	out, ok := current.([]interface{})
	return out, ok, nil
}

func unstructuredMap(obj interface{}, field string) (map[string]interface{}, bool, error) {
	m, ok := obj.(map[string]interface{})
	if !ok {
		return nil, false, nil
	}
	out, ok := m[field].(map[string]interface{})
	return out, ok, nil
}
