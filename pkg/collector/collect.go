// Package collector fetches Kubernetes resources for the inventory scan.
//
// # Fast mode (default)
//
// Uses the metadata client (PartialObjectMetadataList) for every resource type.
// This is significantly faster than fetching full objects because payloads are
// smaller. GitOps detection, finalizer state, owner graph, and stuck detection
// all work from metadata alone.
//
// A small set of "deep Kinds" (Pods, Deployments, Ingresses, etc.) are always
// fully hydrated via the dynamic client because the reference-walker needs their
// spec to build cross-resource relationships (e.g. Deployment → Secret via envFrom).
//
// # Deep mode (--deep flag)
//
// All resources are fetched as full Unstructured objects. Use this when you need
// complete reference analysis across CRDs or custom operators.
package collector

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shahneil76/kubectl-inventory/pkg/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/metadata"
)

// deepKinds are always fetched as full Unstructured objects even in fast mode,
// because the reference-walker needs their spec for accurate cross-resource analysis.
var deepKinds = map[string]bool{
	"Pod":                    true,
	"Deployment":             true,
	"ReplicaSet":             true,
	"StatefulSet":            true,
	"DaemonSet":              true,
	"Job":                    true,
	"CronJob":                true,
	"Ingress":                true,
	"HTTPRoute":              true,
	"Service":                true,
	"HorizontalPodAutoscaler": true,
	"RoleBinding":            true,
	"ClusterRoleBinding":     true,
	"ConfigMap":              true,
	"Secret":                 true,
	"ServiceAccount":         true,
	"LimitRange":             true,
	"PodDisruptionBudget":    true,
}

// Options controls collection behaviour.
type Options struct {
	Namespace      string
	AllNamespaces  bool
	IncludeSystem  bool
	Age            time.Duration
	Concurrency    int           // max concurrent list goroutines; default 10
	RequestTimeout time.Duration // per-GVR context timeout; 0 = no per-request timeout
	Selector       string        // label selector passed to every List call
	Deep           bool          // fetch full objects for all resource types
}

// Result is the output of CollectAll.
type Result struct {
	Resources []types.Resource
	Stats     types.ScanStats
}

// CollectAll fetches all resources in the provided API resource list.
// In fast mode (Deep=false) it uses PartialObjectMetadataList for most types
// and falls back to full Unstructured for deepKinds and any type that the
// metadata client cannot serve.
func CollectAll(
	ctx context.Context,
	dynamicClient dynamic.Interface,
	metaClient metadata.Interface,
	namespace string,
	allNamespaces bool,
	apiResources []types.APIResource,
	opts Options,
) (Result, error) {
	started := time.Now()

	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = 10
	}

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var resources []types.Resource
	var skipped []string
	scanned := 0

	for _, apiResource := range apiResources {
		apiResource := apiResource
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			// Derive per-GVR context.
			listCtx := ctx
			var cancel context.CancelFunc
			if opts.RequestTimeout > 0 {
				listCtx, cancel = context.WithTimeout(ctx, opts.RequestTimeout)
				defer cancel()
			}

			useFull := opts.Deep || deepKinds[apiResource.Kind]

			var items []types.Resource
			var err error
			if useFull {
				items, err = listFull(listCtx, dynamicClient, namespace, allNamespaces, apiResource, opts.Selector)
			} else {
				items, err = listMeta(listCtx, metaClient, namespace, allNamespaces, apiResource, opts.Selector)
				if err != nil && isMetadataUnsupported(err) {
					// Some aggregated or custom APIs don't support the metadata API.
					// Fall back to a full dynamic list.
					items, err = listFull(listCtx, dynamicClient, namespace, allNamespaces, apiResource, opts.Selector)
				}
			}

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				skipped = append(skipped, fmt.Sprintf("%s: %v", apiResource.DisplayName(), err))
				return
			}
			scanned++
			resources = append(resources, items...)
		}()
	}
	wg.Wait()

	BuildOwnerGraph(resources)

	return Result{
		Resources: resources,
		Stats: types.ScanStats{
			DiscoveredTypes: len(apiResources),
			ScannedTypes:    scanned,
			SkippedTypes:    len(skipped),
			SkippedReasons:  skipped,
			Duration:        time.Since(started),
		},
	}, nil
}

// listMeta fetches PartialObjectMetadata for a GVR. Labels, annotations,
// ownerRefs, finalizers, and deletionTimestamp are all present.
func listMeta(
	ctx context.Context,
	metaClient metadata.Interface,
	namespace string,
	allNamespaces bool,
	apiResource types.APIResource,
	selector string,
) ([]types.Resource, error) {
	gvrSchema := gvr(apiResource)
	ri := metaClient.Resource(gvrSchema)

	listOpts := metav1.ListOptions{}
	if selector != "" {
		listOpts.LabelSelector = selector
	}

	var list *metav1.PartialObjectMetadataList
	var err error
	if apiResource.Namespaced {
		if allNamespaces {
			list, err = ri.Namespace("").List(ctx, listOpts)
		} else {
			list, err = ri.Namespace(namespace).List(ctx, listOpts)
		}
	} else {
		list, err = ri.Namespace("").List(ctx, listOpts)
	}
	if err != nil {
		return nil, err
	}

	items := make([]types.Resource, 0, len(list.Items))
	for i := range list.Items {
		r := types.NewResourceFromMeta(apiResource, &list.Items[i])
		DetectGitOpsFromMeta(&r, list.Items[i].GetLabels(), list.Items[i].GetAnnotations())
		items = append(items, r)
	}
	return items, nil
}

// listFull fetches complete Unstructured objects for a GVR.
func listFull(
	ctx context.Context,
	dynamicClient dynamic.Interface,
	namespace string,
	allNamespaces bool,
	apiResource types.APIResource,
	selector string,
) ([]types.Resource, error) {
	rc := dynamicClient.Resource(gvr(apiResource))
	listOpts := metav1.ListOptions{}
	if selector != "" {
		listOpts.LabelSelector = selector
	}

	var list *unstructured.UnstructuredList
	var err error
	if apiResource.Namespaced {
		if allNamespaces {
			list, err = rc.List(ctx, listOpts)
		} else {
			list, err = rc.Namespace(namespace).List(ctx, listOpts)
		}
	} else {
		list, err = rc.List(ctx, listOpts)
	}
	if err != nil {
		return nil, err
	}

	items := make([]types.Resource, 0, len(list.Items))
	for i := range list.Items {
		obj := &list.Items[i]
		r := types.NewResource(apiResource, obj)
		DetectGitOps(&r)
		MarkFinalizerState(&r)
		SetCreatedBy(&r)
		items = append(items, r)
	}
	return items, nil
}

// isMetadataUnsupported returns true for errors that indicate the metadata API
// is not available for this particular GVR (e.g. aggregated APIs that don't
// speak the standard metadata accept header).
func isMetadataUnsupported(err error) bool {
	if err == nil {
		return false
	}
	return apierrors.IsNotAcceptable(err) ||
		apierrors.IsUnsupportedMediaType(err) ||
		apierrors.IsMethodNotSupported(err)
}

func gvr(apiResource types.APIResource) schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    apiResource.Group,
		Version:  apiResource.Version,
		Resource: apiResource.Resource,
	}
}
