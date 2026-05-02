package discovery

import (
	"fmt"
	"strings"

	"github.com/shahneil76/kubectl-inventory/pkg/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
)

func DiscoverAllResources(discoveryClient discovery.DiscoveryInterface) ([]types.APIResource, []string, error) {
	lists, err := discoveryClient.ServerPreferredResources()
	warnings := []string{}
	if err != nil {
		if groupErr, ok := err.(*discovery.ErrGroupDiscoveryFailed); ok {
			for gv, groupErr := range groupErr.Groups {
				warnings = append(warnings, fmt.Sprintf("discovery failed for %s: %v", gv.String(), groupErr))
			}
		} else if apierrors.IsForbidden(err) || apierrors.IsNotFound(err) {
			warnings = append(warnings, fmt.Sprintf("partial discovery failure: %v", err))
		} else {
			return nil, warnings, err
		}
	}

	seen := map[string]bool{}
	resources := []types.APIResource{}
	for _, list := range lists {
		groupVersion, err := parseGroupVersion(list.GroupVersion)
		if err != nil {
			warnings = append(warnings, err.Error())
			continue
		}

		for _, resource := range list.APIResources {
			if !supportsVerb(resource, "list") || strings.Contains(resource.Name, "/") {
				continue
			}
			apiResource := types.APIResource{
				Group:      groupVersion.Group,
				Version:    groupVersion.Version,
				Resource:   resource.Name,
				Kind:       resource.Kind,
				Namespaced: resource.Namespaced,
			}
			key := apiResource.GroupVersionResource()
			if seen[key] {
				continue
			}
			seen[key] = true
			resources = append(resources, apiResource)
		}
	}

	return resources, warnings, nil
}

func supportsVerb(resource metav1.APIResource, verb string) bool {
	for _, supported := range resource.Verbs {
		if supported == verb {
			return true
		}
	}
	return false
}

type groupVersion struct {
	Group   string
	Version string
}

func parseGroupVersion(value string) (groupVersion, error) {
	parts := strings.Split(value, "/")
	if len(parts) == 1 {
		return groupVersion{Version: parts[0]}, nil
	}
	if len(parts) == 2 {
		return groupVersion{Group: parts[0], Version: parts[1]}, nil
	}
	return groupVersion{}, fmt.Errorf("invalid groupVersion %q", value)
}
