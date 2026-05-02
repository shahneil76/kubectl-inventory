package types

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8stypes "k8s.io/apimachinery/pkg/types"
)

const (
	GitOpsNone = ""
	GitOpsArgo = "ArgoCD"
	GitOpsFlux = "Flux"
	GitOpsHelm = "Helm"

	OrphanStatusUnknown    = "unknown"
	OrphanStatusOwned      = "owned"
	OrphanStatusDangling   = "dangling" // has ownerReferences but ALL owners are gone from the cluster
	OrphanStatusManaged    = "managed"
	OrphanStatusGenerated  = "generated"
	OrphanStatusReferenced = "referenced"
	OrphanStatusStandalone = "standalone"
	OrphanStatusSuspicious = "suspicious"
)

type APIResource struct {
	Group      string `json:"group"`
	Version    string `json:"version"`
	Resource   string `json:"resource"`
	Kind       string `json:"kind"`
	Namespaced bool   `json:"namespaced"`
}

func (r APIResource) GroupVersionResource() string {
	if r.Group == "" {
		return r.Version + "/" + r.Resource
	}
	return r.Group + "/" + r.Version + "/" + r.Resource
}

func (r APIResource) DisplayName() string {
	if r.Group == "" {
		return r.Resource
	}
	return r.Resource + "." + r.Group
}

type OwnerRef struct {
	APIVersion string       `json:"apiVersion"`
	Kind       string       `json:"kind"`
	Name       string       `json:"name"`
	UID        k8stypes.UID `json:"uid"`
}

type Resource struct {
	Group     string       `json:"group"`
	Version   string       `json:"version"`
	Resource  string       `json:"resource"`
	Kind      string       `json:"kind"`
	Name      string       `json:"name"`
	Namespace string       `json:"namespace,omitempty"`
	UID       k8stypes.UID `json:"uid"`

	OwnerRefs     []OwnerRef `json:"ownerRefs,omitempty"`
	IsOwned       bool       `json:"isOwned"`
	IsReferenced  bool       `json:"isReferenced"`
	IsOrphaned    bool       `json:"isOrphaned"` // Backwards-compatible alias for suspicious orphan candidates.
	OrphanStatus  string     `json:"orphanStatus,omitempty"`
	OrphanReasons []string   `json:"orphanReasons,omitempty"`

	GitOpsTool string `json:"gitOpsTool,omitempty"`
	AppName    string `json:"appName,omitempty"`
	SourcePath string `json:"sourcePath,omitempty"`

	IsStuck    bool          `json:"isStuck"`
	Finalizers []string      `json:"finalizers,omitempty"`
	CreatedAt  time.Time     `json:"createdAt"`
	CreatedBy  string        `json:"createdBy,omitempty"`
	Age        time.Duration `json:"age"`

	Raw *unstructured.Unstructured `json:"raw,omitempty"`
}

func NewResource(api APIResource, obj *unstructured.Unstructured) Resource {
	ownerRefs := make([]OwnerRef, 0, len(obj.GetOwnerReferences()))
	for _, ref := range obj.GetOwnerReferences() {
		ownerRefs = append(ownerRefs, OwnerRef{
			APIVersion: ref.APIVersion,
			Kind:       ref.Kind,
			Name:       ref.Name,
			UID:        ref.UID,
		})
	}

	createdAt := obj.GetCreationTimestamp().Time
	return Resource{
		Group:      api.Group,
		Version:    api.Version,
		Resource:   api.Resource,
		Kind:       api.Kind,
		Name:       obj.GetName(),
		Namespace:  obj.GetNamespace(),
		UID:        obj.GetUID(),
		OwnerRefs:  ownerRefs,
		IsOwned:    len(ownerRefs) > 0,
		Finalizers: obj.GetFinalizers(),
		CreatedAt:  createdAt,
		Age:        time.Since(createdAt),
		Raw:        obj,
	}
}

// NewResourceFromMeta builds a Resource from a PartialObjectMetadata.
// Raw is intentionally left nil — reference-walking (buildReferences) is
// skipped in fast mode since it requires full spec payloads.
func NewResourceFromMeta(api APIResource, obj *metav1.PartialObjectMetadata) Resource {
	ownerRefs := make([]OwnerRef, 0, len(obj.GetOwnerReferences()))
	for _, ref := range obj.GetOwnerReferences() {
		ownerRefs = append(ownerRefs, OwnerRef{
			APIVersion: ref.APIVersion,
			Kind:       ref.Kind,
			Name:       ref.Name,
			UID:        ref.UID,
		})
	}

	createdAt := obj.GetCreationTimestamp().Time

	var managedBy string
	for _, f := range obj.GetManagedFields() {
		if f.Manager != "" {
			managedBy = f.Manager
			break
		}
	}

	finalizers := obj.GetFinalizers()
	isStuck := obj.GetDeletionTimestamp() != nil && len(finalizers) > 0

	return Resource{
		Group:     api.Group,
		Version:   api.Version,
		Resource:  api.Resource,
		Kind:      api.Kind,
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
		UID:       obj.GetUID(),
		OwnerRefs: ownerRefs,
		IsOwned:   len(ownerRefs) > 0,
		Finalizers: finalizers,
		IsStuck:   isStuck,
		CreatedAt: createdAt,
		Age:       time.Since(createdAt),
		CreatedBy: managedBy,
		Raw:       nil, // fast mode: no spec
	}
}

type ScanStats struct {
	DiscoveredTypes int           `json:"discoveredTypes"`
	ScannedTypes    int           `json:"scannedTypes"`
	SkippedTypes    int           `json:"skippedTypes"`
	SkippedReasons  []string      `json:"skippedReasons,omitempty"`
	NoisySkipped    int           `json:"noisySkipped,omitempty"` // excluded by noise profile
	Duration        time.Duration `json:"duration"`
}

type Inventory struct {
	Context       string     `json:"context,omitempty"`
	Namespace     string     `json:"namespace,omitempty"`
	AllNamespaces bool       `json:"allNamespaces"`
	Resources     []Resource `json:"resources"`
	Stats         ScanStats  `json:"stats"`
}

func OwnerRefsFromMeta(refs []metav1.OwnerReference) []OwnerRef {
	out := make([]OwnerRef, 0, len(refs))
	for _, ref := range refs {
		out = append(out, OwnerRef{APIVersion: ref.APIVersion, Kind: ref.Kind, Name: ref.Name, UID: ref.UID})
	}
	return out
}
