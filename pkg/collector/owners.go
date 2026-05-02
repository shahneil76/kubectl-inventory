package collector

import "github.com/shahneil76/kubectl-inventory/pkg/types"

func BuildOwnerGraph(resources []types.Resource) map[string]*types.Resource {
	byUID := map[string]*types.Resource{}
	for i := range resources {
		byUID[string(resources[i].UID)] = &resources[i]
	}

	for i := range resources {
		resources[i].IsOwned = false
		for _, owner := range resources[i].OwnerRefs {
			if _, ok := byUID[string(owner.UID)]; ok {
				resources[i].IsOwned = true
				break
			}
		}
		if len(resources[i].OwnerRefs) > 0 && !resources[i].IsOwned {
			resources[i].IsOwned = true
		}
	}

	return byUID
}
