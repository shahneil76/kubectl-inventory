package collector

import (
	"strings"

	"github.com/shahneil76/kubectl-inventory/pkg/types"
)

// DetectGitOps detects GitOps tooling for resources fetched with a full object
// (Raw != nil). It delegates to DetectGitOpsFromMeta after extracting labels
// and annotations from the raw object.
func DetectGitOps(resource *types.Resource) {
	if resource.Raw == nil {
		return
	}
	DetectGitOpsFromMeta(resource, resource.Raw.GetLabels(), resource.Raw.GetAnnotations())
}

// DetectGitOpsFromMeta detects GitOps tooling from pre-extracted label and
// annotation maps. It is called from both the full-object path (via DetectGitOps)
// and the fast metadata-only path where Raw is nil.
func DetectGitOpsFromMeta(resource *types.Resource, labels, annotations map[string]string) {
	if trackingID := annotations["argocd.argoproj.io/tracking-id"]; trackingID != "" {
		resource.GitOpsTool = types.GitOpsArgo
		resource.AppName = strings.Split(trackingID, ":")[0]
		resource.SourcePath = annotations["argocd.argoproj.io/manifest-generate-paths"]
		return
	}

	if isHelmManaged(resource.Kind, labels, annotations, "") {
		resource.GitOpsTool = types.GitOpsHelm
		resource.AppName = firstNonEmpty(labels["app.kubernetes.io/instance"], annotations["meta.helm.sh/release-name"], labels["name"])
		return
	}

	if name := annotations["kustomize.toolkit.fluxcd.io/name"]; name != "" {
		resource.GitOpsTool = types.GitOpsFlux
		resource.AppName = name
		resource.SourcePath = annotations["kustomize.toolkit.fluxcd.io/path"]
		return
	}

	if name := annotations["helm.toolkit.fluxcd.io/name"]; name != "" {
		resource.GitOpsTool = types.GitOpsFlux
		resource.AppName = name
		return
	}
}

func isHelmManaged(kind string, labels, annotations map[string]string, gvkKind string) bool {
	if labels["app.kubernetes.io/managed-by"] == "Helm" {
		return true
	}
	if labels["helm.sh/chart"] != "" {
		return true
	}
	if annotations["meta.helm.sh/release-name"] != "" || annotations["meta.helm.sh/release-namespace"] != "" {
		return true
	}
	if kind == "Secret" && labels["owner"] == "helm" {
		return true
	}
	if gvkKind == "Secret" && labels["owner"] == "helm" {
		return true
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
