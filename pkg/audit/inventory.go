package audit

import (
	"reflect"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/shahneil76/kubectl-inventory/pkg/types"
)

// RunFromInventory builds audit input from a scanned inventory and runs checks.
func RunFromInventory(resources []types.Resource, namespace string) *ScanResults {
	return RunPostureFromInventory(resources, namespace).ToScanResults()
}

// RunPostureFromInventory is the inventory-native entry point: best-practice checks
// plus dangling/stuck/GitOps-drift signals and posture scoring.
func RunPostureFromInventory(resources []types.Resource, namespace string) *PostureReport {
	input := &CheckInput{}
	nsFilter := namespace != "" && namespace != "*"

	for _, res := range resources {
		if nsFilter && res.Namespace != namespace {
			continue
		}
		appendResource(input, res)
	}

	scan := RunChecks(input)
	return BuildPostureReport(resources, scan, namespace)
}

func appendResource(input *CheckInput, res types.Resource) {
	if res.Raw != nil {
		appendFromUnstructured(input, res.Raw)
		return
	}
	appendFromMeta(input, res)
}

func appendFromUnstructured(input *CheckInput, obj *unstructured.Unstructured) {
	switch obj.GetKind() {
	case "Pod":
		if p := convert[*corev1.Pod](obj); p != nil {
			input.Pods = append(input.Pods, p)
		}
	case "Deployment":
		if d := convert[*appsv1.Deployment](obj); d != nil {
			input.Deployments = append(input.Deployments, d)
		}
	case "StatefulSet":
		if s := convert[*appsv1.StatefulSet](obj); s != nil {
			input.StatefulSets = append(input.StatefulSets, s)
		}
	case "DaemonSet":
		if d := convert[*appsv1.DaemonSet](obj); d != nil {
			input.DaemonSets = append(input.DaemonSets, d)
		}
	case "Service":
		if s := convert[*corev1.Service](obj); s != nil {
			input.Services = append(input.Services, s)
		}
	case "Ingress":
		if i := convert[*networkingv1.Ingress](obj); i != nil {
			input.Ingresses = append(input.Ingresses, i)
		}
	case "HorizontalPodAutoscaler":
		if h := convert[*autoscalingv2.HorizontalPodAutoscaler](obj); h != nil {
			input.HorizontalPodAutoscalers = append(input.HorizontalPodAutoscalers, h)
		} else if h1 := convert[*autoscalingv1.HorizontalPodAutoscaler](obj); h1 != nil {
			input.HorizontalPodAutoscalers = append(input.HorizontalPodAutoscalers, hpaV1ToV2(h1))
		}
	case "PodDisruptionBudget":
		if p := convert[*policyv1.PodDisruptionBudget](obj); p != nil {
			input.PodDisruptionBudgets = append(input.PodDisruptionBudgets, p)
		}
	case "ConfigMap":
		if c := convert[*corev1.ConfigMap](obj); c != nil {
			input.ConfigMaps = append(input.ConfigMaps, c)
		}
	case "Secret":
		if s := convert[*corev1.Secret](obj); s != nil {
			input.Secrets = append(input.Secrets, s)
		}
	case "ServiceAccount":
		if sa := convert[*corev1.ServiceAccount](obj); sa != nil {
			input.ServiceAccounts = append(input.ServiceAccounts, sa)
		}
	case "LimitRange":
		if lr := convert[*corev1.LimitRange](obj); lr != nil {
			input.LimitRanges = append(input.LimitRanges, lr)
		}
	default:
		if isCrossplaneMR(obj) || isCrossplaneComposite(obj) {
			if isCrossplaneMR(obj) {
				input.ManagedResources = append(input.ManagedResources, obj)
			} else {
				input.CompositeResources = append(input.CompositeResources, obj)
			}
		}
	}
}

func appendFromMeta(input *CheckInput, res types.Resource) {
	meta := metaFromResource(res)
	switch res.Kind {
	case "Pod":
		input.Pods = append(input.Pods, &corev1.Pod{ObjectMeta: meta})
	case "Deployment":
		input.Deployments = append(input.Deployments, &appsv1.Deployment{ObjectMeta: meta})
	case "StatefulSet":
		input.StatefulSets = append(input.StatefulSets, &appsv1.StatefulSet{ObjectMeta: meta})
	case "DaemonSet":
		input.DaemonSets = append(input.DaemonSets, &appsv1.DaemonSet{ObjectMeta: meta})
	case "Service":
		input.Services = append(input.Services, &corev1.Service{ObjectMeta: meta})
	case "Ingress":
		input.Ingresses = append(input.Ingresses, &networkingv1.Ingress{ObjectMeta: meta})
	case "HorizontalPodAutoscaler":
		input.HorizontalPodAutoscalers = append(input.HorizontalPodAutoscalers, &autoscalingv2.HorizontalPodAutoscaler{ObjectMeta: meta})
	case "PodDisruptionBudget":
		input.PodDisruptionBudgets = append(input.PodDisruptionBudgets, &policyv1.PodDisruptionBudget{ObjectMeta: meta})
	case "ConfigMap":
		input.ConfigMaps = append(input.ConfigMaps, &corev1.ConfigMap{ObjectMeta: meta})
	case "Secret":
		input.Secrets = append(input.Secrets, &corev1.Secret{ObjectMeta: meta})
	case "ServiceAccount":
		input.ServiceAccounts = append(input.ServiceAccounts, &corev1.ServiceAccount{ObjectMeta: meta})
	case "LimitRange":
		input.LimitRanges = append(input.LimitRanges, &corev1.LimitRange{ObjectMeta: meta})
	}
}

func metaFromResource(res types.Resource) metav1.ObjectMeta {
	meta := metav1.ObjectMeta{
		Name:              res.Name,
		Namespace:         res.Namespace,
		UID:               res.UID,
		Finalizers:        res.Finalizers,
		CreationTimestamp: metav1.NewTime(res.CreatedAt),
	}
	if len(res.OwnerRefs) > 0 {
		meta.OwnerReferences = make([]metav1.OwnerReference, 0, len(res.OwnerRefs))
		for _, ref := range res.OwnerRefs {
			meta.OwnerReferences = append(meta.OwnerReferences, metav1.OwnerReference{
				APIVersion: ref.APIVersion,
				Kind:       ref.Kind,
				Name:       ref.Name,
				UID:        ref.UID,
			})
		}
	}
	if res.IsStuck {
		now := metav1.Now()
		meta.DeletionTimestamp = &now
	}
	return meta
}

func convert[T runtime.Object](obj *unstructured.Unstructured) T {
	var zero T
	if obj == nil || obj.Object == nil {
		return zero
	}
	rt := reflect.TypeOf(zero)
	if rt == nil || rt.Kind() != reflect.Ptr || rt.Elem() == nil {
		return zero
	}
	outVal := reflect.New(rt.Elem())
	out, ok := outVal.Interface().(T)
	if !ok {
		return zero
	}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, out); err != nil {
		return zero
	}
	return out
}

func isCrossplaneMR(u *unstructured.Unstructured) bool {
	spec, ok := u.Object["spec"].(map[string]interface{})
	if !ok {
		return false
	}
	if _, ok := spec["providerConfigRef"].(map[string]interface{}); ok {
		return true
	}
	if cp, ok := spec["crossplane"].(map[string]interface{}); ok {
		if _, ok := cp["providerConfigRef"].(map[string]interface{}); ok {
			return true
		}
	}
	return false
}

func isCrossplaneComposite(u *unstructured.Unstructured) bool {
	if isCrossplaneMR(u) {
		return false
	}
	spec, ok := u.Object["spec"].(map[string]interface{})
	if !ok {
		return false
	}
	if _, ok := spec["resourceRefs"].([]interface{}); ok {
		return true
	}
	if cp, ok := spec["crossplane"].(map[string]interface{}); ok {
		if _, ok := cp["resourceRefs"].([]interface{}); ok {
			return true
		}
	}
	if _, hasRef := spec["resourceRef"].(map[string]interface{}); hasRef {
		if _, hasComp := spec["compositionRef"].(map[string]interface{}); hasComp {
			return true
		}
	}
	return false
}

func hpaV1ToV2(h1 *autoscalingv1.HorizontalPodAutoscaler) *autoscalingv2.HorizontalPodAutoscaler {
	if h1 == nil {
		return nil
	}
	return &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: h1.ObjectMeta,
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				Kind:       h1.Spec.ScaleTargetRef.Kind,
				Name:       h1.Spec.ScaleTargetRef.Name,
				APIVersion: h1.Spec.ScaleTargetRef.APIVersion,
			},
			MinReplicas: h1.Spec.MinReplicas,
			MaxReplicas: h1.Spec.MaxReplicas,
		},
	}
}
