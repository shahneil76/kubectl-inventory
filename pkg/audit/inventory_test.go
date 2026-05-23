package audit

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/shahneil76/kubectl-inventory/pkg/types"
)

func TestConvertServiceDoesNotPanic(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      "web",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"selector": map[string]interface{}{"app": "web"},
				"ports": []interface{}{
					map[string]interface{}{"port": int64(80), "targetPort": int64(8080)},
				},
			},
		},
	}

	svc := convert[*corev1.Service](obj)
	if svc == nil {
		t.Fatal("expected converted Service, got nil")
	}
	if svc.Name != "web" || svc.Namespace != "default" {
		t.Fatalf("unexpected service identity: %s/%s", svc.Namespace, svc.Name)
	}
}

func TestConvertReturnsZeroOnFailure(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"spec":       "not-a-map",
		},
	}

	svc := convert[*corev1.Service](obj)
	if svc != nil {
		t.Fatalf("expected nil on conversion failure, got %#v", svc)
	}
}

func TestRunFromInventoryWithServices(t *testing.T) {
	raw := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      "orphan",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"selector": map[string]interface{}{"app": "missing"},
				"ports": []interface{}{
					map[string]interface{}{"port": int64(80)},
				},
			},
		},
	}

	resources := []types.Resource{
		{
			Group:     "",
			Version:   "v1",
			Resource:  "services",
			Kind:      "Service",
			Name:      "orphan",
			Namespace: "default",
			UID:       "uid-1",
			Raw:       raw,
		},
	}

	results := RunFromInventory(resources, "*")
	if results == nil {
		t.Fatal("expected scan results")
	}
	if len(results.Checks) == 0 {
		t.Fatal("expected check registry in results")
	}
}

func TestMetaFromResourceStuck(t *testing.T) {
	meta := metaFromResource(types.Resource{
		Name:      "stuck-pod",
		Namespace: "default",
		IsStuck:   true,
	})
	if meta.DeletionTimestamp == nil {
		t.Fatal("expected deletion timestamp for stuck resource")
	}
}

func TestConvertNilObject(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("convert panicked on nil object: %v", r)
		}
	}()
	svc := convert[*corev1.Service](nil)
	if svc != nil {
		t.Fatalf("expected nil, got %#v", svc)
	}
}

func TestConvertTypedObjectRoundTrip(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "kube-system"},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{Port: 443}},
		},
	}
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(svc)
	if err != nil {
		t.Fatal(err)
	}
	out := convert[*corev1.Service](&unstructured.Unstructured{Object: u})
	if out == nil || out.Name != "api" {
		t.Fatalf("round-trip failed: %#v", out)
	}
}
