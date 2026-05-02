package analyzer

import (
	"fmt"
	"strings"
	"time"

	"github.com/shahneil76/kubectl-inventory/pkg/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type resourceIndex map[string]int

type referenceInfo struct {
	Referenced bool
	Reasons    []string
}

func MarkOrphans(resources []types.Resource, minAge time.Duration) []types.Resource {
	idx := indexResources(resources)
	uidSet := buildUIDSet(resources)
	references := buildReferences(resources, idx)
	candidates := []types.Resource{}

	for i := range resources {
		status, reasons := classifyResource(resources[i], idx, uidSet, references[string(resources[i].UID)], minAge)
		resources[i].OrphanStatus = status
		resources[i].OrphanReasons = reasons
		resources[i].IsReferenced = references[string(resources[i].UID)].Referenced
		resources[i].IsOrphaned = status == types.OrphanStatusSuspicious || status == types.OrphanStatusDangling
		if resources[i].IsOrphaned {
			candidates = append(candidates, resources[i])
		}
	}

	return candidates
}

func classifyResource(resource types.Resource, idx resourceIndex, uidSet map[string]struct{}, ref referenceInfo, minAge time.Duration) (string, []string) {
	reasons := []string{}

	// Universal check: ownerReferences exist but ALL owners are gone from the cluster.
	// This works for any Kind — CRDs, built-in, anything — without needing to know what
	// the Kind is. A dangling resource is usually a stronger signal than a suspicious one.
	if len(resource.OwnerRefs) > 0 {
		if dangling, danglingReasons := brokenOwnerRefs(resource, uidSet); dangling {
			return types.OrphanStatusDangling, danglingReasons
		}
		return types.OrphanStatusOwned, []string{"has ownerReferences"}
	}

	if resource.GitOpsTool != "" {
		return types.OrphanStatusManaged, []string{fmt.Sprintf("managed by %s", resource.GitOpsTool)}
	}

	if isWellKnownSystemResource(resource) {
		return types.OrphanStatusStandalone, []string{"well-known Kubernetes/system resource"}
	}

	if generated, reason := isKnownGeneratedResource(resource, idx); generated {
		return types.OrphanStatusGenerated, []string{reason}
	}

	if ref.Referenced {
		return types.OrphanStatusReferenced, ref.Reasons
	}

	if minAge > 0 && resource.Age < minAge {
		return types.OrphanStatusStandalone, []string{fmt.Sprintf("younger than --age threshold (%s)", minAge)}
	}

	if !isSuspiciousCandidateKind(resource) {
		reasons = append(reasons, "top-level resource kind; no strong orphan signal")
		if resource.Group != "" {
			reasons = append(reasons, "custom/API-group resource may be intentionally standalone")
		}
		return types.OrphanStatusStandalone, reasons
	}

	reasons = append(reasons, "no ownerReferences")
	reasons = append(reasons, "no known GitOps/controller metadata detected")
	reasons = append(reasons, "not referenced by scanned resources")
	if resource.CreatedBy != "" {
		reasons = append(reasons, "created/managed by "+resource.CreatedBy)
	}
	return types.OrphanStatusSuspicious, reasons
}

func indexResources(resources []types.Resource) resourceIndex {
	idx := resourceIndex{}
	for i, resource := range resources {
		idx[resourceKey(resource.Kind, resource.Namespace, resource.Name)] = i
	}
	return idx
}

// buildUIDSet builds a set of all scanned resource UIDs for fast owner-ref lookups.
func buildUIDSet(resources []types.Resource) map[string]struct{} {
	set := make(map[string]struct{}, len(resources))
	for _, r := range resources {
		if r.UID != "" {
			set[string(r.UID)] = struct{}{}
		}
	}
	return set
}

// brokenOwnerRefs returns true when a resource has ownerReferences but ALL referenced
// owners are absent from the current scan. The missing reasons are also returned.
// We only flag as dangling if ALL owners are gone — a single missing owner might be
// cluster-scoped and not captured in a namespaced scan.
func brokenOwnerRefs(resource types.Resource, uidSet map[string]struct{}) (bool, []string) {
	missing := []string{}
	for _, ref := range resource.OwnerRefs {
		if _, exists := uidSet[string(ref.UID)]; !exists {
			missing = append(missing, fmt.Sprintf("%s/%s no longer present in cluster", ref.Kind, ref.Name))
		}
	}
	if len(missing) == len(resource.OwnerRefs) {
		return true, missing
	}
	return false, nil
}

func buildReferences(resources []types.Resource, idx resourceIndex) map[string]referenceInfo {
	references := map[string]referenceInfo{}
	mark := func(kind, namespace, name, reason string) {
		if name == "" {
			return
		}
		pos, ok := idx[resourceKey(kind, namespace, name)]
		if !ok {
			return
		}
		uid := string(resources[pos].UID)
		info := references[uid]
		info.Referenced = true
		info.Reasons = appendUnique(info.Reasons, reason)
		references[uid] = info
	}

	for _, resource := range resources {
		if resource.Raw == nil {
			continue
		}

		switch resource.Kind {
		case "Pod":
			markPodSpecReferences(resource, resource.Raw.Object, mark)
		case "Deployment", "ReplicaSet", "StatefulSet", "DaemonSet", "Job":
			if spec, ok, _ := unstructured.NestedMap(resource.Raw.Object, "spec", "template", "spec"); ok {
				markPodSpecReferences(resource, spec, mark)
			}
			if resource.Kind == "StatefulSet" {
				markStatefulSetPVCs(resource, idx, mark)
			}
		case "CronJob":
			if spec, ok, _ := unstructured.NestedMap(resource.Raw.Object, "spec", "jobTemplate", "spec", "template", "spec"); ok {
				markPodSpecReferences(resource, spec, mark)
			}
		case "Ingress":
			markIngressReferences(resource, mark)
		case "HTTPRoute":
			markHTTPRouteReferences(resource, mark)
		case "RoleBinding", "ClusterRoleBinding":
			markRoleBindingReferences(resource, mark)
		case "HorizontalPodAutoscaler":
			markScaleTargetReferences(resource, mark)
		default:
			// For any resource type the plugin doesn't know specifically, run the generic
			// spec walker. It detects kind+name pairs and common field name patterns so
			// CRDs that reference core resources are not mistakenly flagged as orphans.
			if spec, ok, _ := unstructured.NestedMap(resource.Raw.Object, "spec"); ok {
				markGenericSpecReferences(resource, spec, idx, mark)
			}
		}
	}

	return references
}

func markPodSpecReferences(owner types.Resource, spec map[string]interface{}, mark func(kind, namespace, name, reason string)) {
	reason := fmt.Sprintf("referenced by %s/%s", owner.Kind, owner.Name)

	for _, secretName := range nestedStringSliceMaps(spec, "imagePullSecrets", "name") {
		mark("Secret", owner.Namespace, secretName, reason+" imagePullSecrets")
	}

	volumes, _, _ := unstructured.NestedSlice(spec, "volumes")
	for _, volume := range volumes {
		v, ok := volume.(map[string]interface{})
		if !ok {
			continue
		}
		if claimName, ok, _ := unstructured.NestedString(v, "persistentVolumeClaim", "claimName"); ok {
			mark("PersistentVolumeClaim", owner.Namespace, claimName, reason+" volume")
		}
		if name, ok, _ := unstructured.NestedString(v, "secret", "secretName"); ok {
			mark("Secret", owner.Namespace, name, reason+" volume")
		}
		if name, ok, _ := unstructured.NestedString(v, "configMap", "name"); ok {
			mark("ConfigMap", owner.Namespace, name, reason+" volume")
		}
		if name, ok, _ := unstructured.NestedString(v, "projected", "sources", "secret", "name"); ok {
			mark("Secret", owner.Namespace, name, reason+" projected volume")
		}
	}

	containers := []interface{}{}
	if values, ok, _ := unstructured.NestedSlice(spec, "containers"); ok {
		containers = append(containers, values...)
	}
	if values, ok, _ := unstructured.NestedSlice(spec, "initContainers"); ok {
		containers = append(containers, values...)
	}
	for _, container := range containers {
		c, ok := container.(map[string]interface{})
		if !ok {
			continue
		}
		for _, envFrom := range nestedMaps(c, "envFrom") {
			if name, ok, _ := unstructured.NestedString(envFrom, "configMapRef", "name"); ok {
				mark("ConfigMap", owner.Namespace, name, reason+" envFrom")
			}
			if name, ok, _ := unstructured.NestedString(envFrom, "secretRef", "name"); ok {
				mark("Secret", owner.Namespace, name, reason+" envFrom")
			}
		}
		for _, env := range nestedMaps(c, "env") {
			if name, ok, _ := unstructured.NestedString(env, "valueFrom", "configMapKeyRef", "name"); ok {
				mark("ConfigMap", owner.Namespace, name, reason+" env")
			}
			if name, ok, _ := unstructured.NestedString(env, "valueFrom", "secretKeyRef", "name"); ok {
				mark("Secret", owner.Namespace, name, reason+" env")
			}
		}
	}
}

func markStatefulSetPVCs(resource types.Resource, idx resourceIndex, mark func(kind, namespace, name, reason string)) {
	templates, ok, _ := unstructured.NestedSlice(resource.Raw.Object, "spec", "volumeClaimTemplates")
	if !ok {
		return
	}
	for _, template := range templates {
		t, ok := template.(map[string]interface{})
		if !ok {
			continue
		}
		claimName, ok, _ := unstructured.NestedString(t, "metadata", "name")
		if !ok || claimName == "" {
			continue
		}
		prefix := claimName + "-" + resource.Name + "-"
		for key := range idx {
			parts := strings.Split(key, "/")
			if len(parts) == 3 && parts[0] == "PersistentVolumeClaim" && parts[1] == resource.Namespace && strings.HasPrefix(parts[2], prefix) {
				mark("PersistentVolumeClaim", resource.Namespace, parts[2], fmt.Sprintf("created from StatefulSet/%s volumeClaimTemplate", resource.Name))
			}
		}
	}
}

func markIngressReferences(resource types.Resource, mark func(kind, namespace, name, reason string)) {
	reason := fmt.Sprintf("referenced by Ingress/%s", resource.Name)
	if serviceName, ok, _ := unstructured.NestedString(resource.Raw.Object, "spec", "defaultBackend", "service", "name"); ok {
		mark("Service", resource.Namespace, serviceName, reason)
	}
	rules, ok, _ := unstructured.NestedSlice(resource.Raw.Object, "spec", "rules")
	if !ok {
		return
	}
	for _, rule := range rules {
		r, ok := rule.(map[string]interface{})
		if !ok {
			continue
		}
		paths, ok, _ := unstructured.NestedSlice(r, "http", "paths")
		if !ok {
			continue
		}
		for _, path := range paths {
			p, ok := path.(map[string]interface{})
			if !ok {
				continue
			}
			if serviceName, ok, _ := unstructured.NestedString(p, "backend", "service", "name"); ok {
				mark("Service", resource.Namespace, serviceName, reason)
			}
		}
	}
}

func markHTTPRouteReferences(resource types.Resource, mark func(kind, namespace, name, reason string)) {
	reason := fmt.Sprintf("referenced by HTTPRoute/%s", resource.Name)
	rules, ok, _ := unstructured.NestedSlice(resource.Raw.Object, "spec", "rules")
	if !ok {
		return
	}
	for _, rule := range rules {
		r, ok := rule.(map[string]interface{})
		if !ok {
			continue
		}
		backendRefs, ok, _ := unstructured.NestedSlice(r, "backendRefs")
		if !ok {
			continue
		}
		for _, backend := range backendRefs {
			b, ok := backend.(map[string]interface{})
			if !ok {
				continue
			}
			name, _ := b["name"].(string)
			kind, _ := b["kind"].(string)
			if kind == "" || kind == "Service" {
				mark("Service", resource.Namespace, name, reason)
			}
		}
	}
}

func markRoleBindingReferences(resource types.Resource, mark func(kind, namespace, name, reason string)) {
	reason := fmt.Sprintf("referenced by %s/%s", resource.Kind, resource.Name)
	if name, ok, _ := unstructured.NestedString(resource.Raw.Object, "roleRef", "name"); ok {
		kind, _, _ := unstructured.NestedString(resource.Raw.Object, "roleRef", "kind")
		if kind == "Role" || kind == "ClusterRole" {
			mark(kind, resource.Namespace, name, reason)
		}
	}
	subjects, ok, _ := unstructured.NestedSlice(resource.Raw.Object, "subjects")
	if !ok {
		return
	}
	for _, subject := range subjects {
		s, ok := subject.(map[string]interface{})
		if !ok {
			continue
		}
		kind, _ := s["kind"].(string)
		name, _ := s["name"].(string)
		namespace, _ := s["namespace"].(string)
		if namespace == "" {
			namespace = resource.Namespace
		}
		if kind == "ServiceAccount" {
			mark("ServiceAccount", namespace, name, reason)
		}
	}
}

func markScaleTargetReferences(resource types.Resource, mark func(kind, namespace, name, reason string)) {
	kind, ok, _ := unstructured.NestedString(resource.Raw.Object, "spec", "scaleTargetRef", "kind")
	if !ok {
		return
	}
	name, ok, _ := unstructured.NestedString(resource.Raw.Object, "spec", "scaleTargetRef", "name")
	if !ok {
		return
	}
	mark(kind, resource.Namespace, name, fmt.Sprintf("referenced by HorizontalPodAutoscaler/%s", resource.Name))
}

// markGenericSpecReferences walks the spec of any resource (typically a CRD) looking for:
//  1. kind+name sibling pairs at any nesting level → marks that (kind, name) as referenced
//  2. well-known field name patterns (secretName, configMapName, etc.) → marks the target
//
// Depth is capped at 8 to keep performance bounded on deeply nested operator specs.
func markGenericSpecReferences(owner types.Resource, spec map[string]interface{}, idx resourceIndex, mark func(kind, namespace, name, reason string)) {
	reason := fmt.Sprintf("referenced by %s/%s", owner.Kind, owner.Name)
	walkSpec(spec, owner.Namespace, reason, idx, mark, 0)
}

// knownRefFields maps spec field names that conventionally hold the name of another resource
// to the Kind of that resource. This is a deliberately small, high-confidence list.
var knownRefFields = map[string]string{
	"secretName":           "Secret",
	"configMapName":        "ConfigMap",
	"serviceName":          "Service",
	"serviceAccountName":   "ServiceAccount",
	"claimName":            "PersistentVolumeClaim",
	"persistentVolumeName": "PersistentVolume",
	"storageClassName":     "StorageClass",
}

func walkSpec(obj map[string]interface{}, namespace, reason string, idx resourceIndex, mark func(kind, namespace, name, reason string), depth int) {
	if depth > 8 {
		return
	}
	// Detect explicit kind+name sibling pairs — common in many CRD backend/ref fields.
	kind, _ := obj["kind"].(string)
	name, _ := obj["name"].(string)
	if kind != "" && name != "" {
		mark(kind, namespace, name, reason)
	}
	// Detect well-known field name patterns.
	for field, targetKind := range knownRefFields {
		if val, ok := obj[field].(string); ok && val != "" {
			mark(targetKind, namespace, val, reason)
		}
	}
	// Recurse.
	for _, v := range obj {
		switch nested := v.(type) {
		case map[string]interface{}:
			walkSpec(nested, namespace, reason, idx, mark, depth+1)
		case []interface{}:
			for _, item := range nested {
				if m, ok := item.(map[string]interface{}); ok {
					walkSpec(m, namespace, reason, idx, mark, depth+1)
				}
			}
		}
	}
}

// isSuspiciousCandidateKind returns true for well-known core Kubernetes "leaf" resource types
// where an unowned, unreferenced, unmanaged instance is genuinely unusual.
// CRD resources (non-empty Group) are deliberately excluded here — an operator's custom resource
// being a root object with no owner refs is completely normal. The only universal signal for
// CRDs is broken ownerRefs (checked earlier in classifyResource via brokenOwnerRefs).
func isSuspiciousCandidateKind(resource types.Resource) bool {
	if resource.Group != "" {
		// Custom / operator resource: never flag as suspicious based on absence of refs alone.
		return false
	}
	switch resource.Kind {
	case "Pod", "ConfigMap", "Secret", "PersistentVolumeClaim", "Endpoints":
		return true
	default:
		return false
	}
}

func isKnownGeneratedResource(resource types.Resource, idx resourceIndex) (bool, string) {
	labels := labelsOf(resource)
	annotations := annotationsOf(resource)

	if resource.Kind == "Endpoints" {
		if _, ok := idx[resourceKey("Service", resource.Namespace, resource.Name)]; ok {
			return true, "generated/maintained for matching Service/" + resource.Name
		}
	}

	if resource.Kind == "EndpointSlice" {
		if labels["kubernetes.io/service-name"] != "" || labels["endpointslice.kubernetes.io/managed-by"] != "" {
			return true, "generated EndpointSlice for Service"
		}
	}

	if resource.Kind == "Secret" {
		secretType := ""
		if resource.Raw != nil {
			secretType, _, _ = unstructured.NestedString(resource.Raw.Object, "type")
		}
		if labels["owner"] == "helm" || secretType == "helm.sh/release.v1" {
			return true, "Helm release metadata secret"
		}
		if annotations["kubernetes.io/service-account.name"] != "" || secretType == "kubernetes.io/service-account-token" {
			return true, "Kubernetes service-account token secret"
		}
		if annotations["cert-manager.io/certificate-name"] != "" {
			return true, "generated by cert-manager certificate"
		}
		if labels["sealedsecrets.bitnami.com/sealed-secrets-key"] != "" {
			return true, "generated/managed by Bitnami Sealed Secrets"
		}
	}

	if hasKnownControllerSignal(labels, annotations) {
		return true, "known controller/operator metadata detected"
	}

	switch resource.Kind {
	case "Event", "PodMetrics", "CiliumEndpoint", "ControllerRevision", "Lease", "EndpointSlice":
		return true, "known generated/runtime resource kind"
	}

	return false, ""
}

func hasKnownControllerSignal(labels, annotations map[string]string) bool {
	prefixes := []string{
		"serving.knative.dev/",
		"networking.knative.dev/",
		"autoscaling.knative.dev/",
		"duck.knative.dev/",
		"internal.serving.knative.dev/",
		"strimzi.io/",
		"io.cilium/",
		"cert-manager.io/",
		"controller.cert-manager.io/",
		"external-secrets.io/",
		"reconcile.external-secrets.io/",
		"keda.sh/",
		"istio.io/",
		"security.istio.io/",
		"networking.istio.io/",
		"sealedsecrets.bitnami.com/",
	}
	for key := range labels {
		for _, prefix := range prefixes {
			if strings.HasPrefix(key, prefix) {
				return true
			}
		}
	}
	for key := range annotations {
		for _, prefix := range prefixes {
			if strings.HasPrefix(key, prefix) {
				return true
			}
		}
	}
	if labels["app.kubernetes.io/managed-by"] == "strimzi-cluster-operator" || labels["app.kubernetes.io/name"] == "strimzi" || labels["k8s-app"] == "cilium" || labels["istio.io/config"] != "" {
		return true
	}
	return false
}

func isWellKnownSystemResource(resource types.Resource) bool {
	name := resource.Name
	kind := resource.Kind
	if resource.Namespace == "kube-system" || resource.Namespace == "kube-public" || resource.Namespace == "kube-node-lease" {
		return true
	}
	if kind == "ServiceAccount" && name == "default" {
		return true
	}
	if kind == "ConfigMap" && (name == "kube-root-ca.crt" || strings.HasPrefix(name, "istio-ca-root-cert")) {
		return true
	}
	if kind == "Service" && name == "kubernetes" && resource.Namespace == "default" {
		return true
	}
	return false
}

func resourceKey(kind, namespace, name string) string {
	return kind + "/" + namespace + "/" + name
}

func labelsOf(resource types.Resource) map[string]string {
	if resource.Raw == nil {
		return map[string]string{}
	}
	return resource.Raw.GetLabels()
}

func annotationsOf(resource types.Resource) map[string]string {
	if resource.Raw == nil {
		return map[string]string{}
	}
	return resource.Raw.GetAnnotations()
}

func nestedMaps(obj map[string]interface{}, field string) []map[string]interface{} {
	items, ok, _ := unstructured.NestedSlice(obj, field)
	if !ok {
		return nil
	}
	out := []map[string]interface{}{}
	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if ok {
			out = append(out, m)
		}
	}
	return out
}

func nestedStringSliceMaps(obj map[string]interface{}, field, nestedField string) []string {
	items, ok, _ := unstructured.NestedSlice(obj, field)
	if !ok {
		return nil
	}
	out := []string{}
	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		value, _ := m[nestedField].(string)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
