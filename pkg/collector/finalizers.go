package collector

import "github.com/shahneil76/kubectl-inventory/pkg/types"

func MarkFinalizerState(resource *types.Resource) {
	if resource.Raw == nil {
		return
	}
	resource.Finalizers = resource.Raw.GetFinalizers()
	resource.IsStuck = resource.Raw.GetDeletionTimestamp() != nil && len(resource.Finalizers) > 0
}

func SetCreatedBy(resource *types.Resource) {
	if resource.Raw == nil || len(resource.Raw.GetManagedFields()) == 0 {
		return
	}
	for _, field := range resource.Raw.GetManagedFields() {
		if field.Manager != "" {
			resource.CreatedBy = field.Manager
			return
		}
	}
}
