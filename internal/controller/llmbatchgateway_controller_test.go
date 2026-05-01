package controller

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	batchv1alpha1 "github.com/opendatahub-io/llm-d-batch-gateway-operator/api/v1alpha1"
)

func newTestGateway(name, namespace string) *batchv1alpha1.LLMBatchGateway {
	return &batchv1alpha1.LLMBatchGateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: batchv1alpha1.LLMBatchGatewaySpec{
			SecretRef: batchv1alpha1.SecretReference{Name: "test-secrets"},
			DBBackend: "postgresql",
			FileStorage: &batchv1alpha1.FileStorageSpec{
				Type: "s3",
				S3: &batchv1alpha1.S3StorageSpec{
					Region:   "us-east-1",
					Endpoint: "https://s3.example.com",
				},
			},
			APIServer: batchv1alpha1.APIServerSpec{
				Replicas: ptr.To(int32(1)),
				Image:    "test-apiserver:latest",
			},
			Processor: batchv1alpha1.ProcessorSpec{
				Replicas: ptr.To(int32(1)),
				Image:    "test-processor:latest",
				GlobalInferenceGateway: &batchv1alpha1.InferenceGatewaySpec{
					URL:            "http://inference-gw:8000",
					RequestTimeout: "5m",
				},
			},
			GC: batchv1alpha1.GCSpec{
				Image:    "test-gc:latest",
				Interval: "30m",
			},
		},
	}
}

func TestReconcile(t *testing.T) {
	ctx := context.Background()

	helmRenderer, err := NewHelmRenderer("../../batch-gateway/charts/batch-gateway")
	if err != nil {
		t.Fatalf("NewHelmRenderer() error: %v", err)
	}

	reconciler := &LLMBatchGatewayReconciler{
		Client:       k8sClient,
		Scheme:       k8sClient.Scheme(),
		HelmRenderer: helmRenderer,
	}

	t.Run("creates all child resources", func(t *testing.T) {
		gw := newTestGateway("test-create", "default")
		if err := k8sClient.Create(ctx, gw); err != nil {
			t.Fatalf("creating CR: %v", err)
		}
		t.Cleanup(func() {
			_ = k8sClient.Delete(ctx, gw)
		})

		_, err := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: gw.Name, Namespace: gw.Namespace},
		})
		if err != nil {
			t.Fatalf("Reconcile() error: %v", err)
		}

		t.Run("deployments", func(t *testing.T) {
			var deployList appsv1.DeploymentList
			if err := k8sClient.List(ctx, &deployList); err != nil {
				t.Fatalf("listing deployments: %v", err)
			}
			deployCount := 0
			for _, d := range deployList.Items {
				if isOwnedByUID(d.OwnerReferences, gw.UID) {
					deployCount++
				}
			}
			if deployCount != 3 {
				t.Errorf("deployment count = %d, want 3", deployCount)
			}
		})

		t.Run("configmaps", func(t *testing.T) {
			var cmList corev1.ConfigMapList
			if err := k8sClient.List(ctx, &cmList); err != nil {
				t.Fatalf("listing configmaps: %v", err)
			}
			cmCount := 0
			for _, cm := range cmList.Items {
				if isOwnedByUID(cm.OwnerReferences, gw.UID) {
					cmCount++
				}
			}
			if cmCount < 3 {
				t.Errorf("configmap count = %d, want >= 3", cmCount)
			}
		})

		t.Run("service accounts", func(t *testing.T) {
			var saList corev1.ServiceAccountList
			if err := k8sClient.List(ctx, &saList); err != nil {
				t.Fatalf("listing serviceaccounts: %v", err)
			}
			saCount := 0
			for _, sa := range saList.Items {
				if isOwnedByUID(sa.OwnerReferences, gw.UID) {
					saCount++
				}
			}
			if saCount != 3 {
				t.Errorf("serviceaccount count = %d, want 3", saCount)
			}
		})

		t.Run("service", func(t *testing.T) {
			var svcList corev1.ServiceList
			if err := k8sClient.List(ctx, &svcList); err != nil {
				t.Fatalf("listing services: %v", err)
			}
			svcCount := 0
			for _, svc := range svcList.Items {
				if isOwnedByUID(svc.OwnerReferences, gw.UID) {
					svcCount++
				}
			}
			if svcCount != 1 {
				t.Errorf("service count = %d, want 1", svcCount)
			}
		})
	})

	t.Run("sets owner references", func(t *testing.T) {
		gw := newTestGateway("test-owner", "default")
		if err := k8sClient.Create(ctx, gw); err != nil {
			t.Fatalf("creating CR: %v", err)
		}
		t.Cleanup(func() {
			_ = k8sClient.Delete(ctx, gw)
		})

		_, err := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: gw.Name, Namespace: gw.Namespace},
		})
		if err != nil {
			t.Fatalf("Reconcile() error: %v", err)
		}

		var deployList appsv1.DeploymentList
		if err := k8sClient.List(ctx, &deployList); err != nil {
			t.Fatalf("listing deployments: %v", err)
		}

		for _, d := range deployList.Items {
			if !isOwnedByUID(d.OwnerReferences, gw.UID) {
				continue
			}
			found := false
			for _, ref := range d.OwnerReferences {
				if ref.UID == gw.UID && ref.Kind == "LLMBatchGateway" && ref.Controller != nil && *ref.Controller {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("deployment %s missing controller owner reference", d.Name)
			}
		}
	})

	t.Run("sets status conditions", func(t *testing.T) {
		gw := newTestGateway("test-status", "default")
		if err := k8sClient.Create(ctx, gw); err != nil {
			t.Fatalf("creating CR: %v", err)
		}
		t.Cleanup(func() {
			_ = k8sClient.Delete(ctx, gw)
		})

		_, err := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: gw.Name, Namespace: gw.Namespace},
		})
		if err != nil {
			t.Fatalf("Reconcile() error: %v", err)
		}

		var updated batchv1alpha1.LLMBatchGateway
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: gw.Name, Namespace: gw.Namespace}, &updated); err != nil {
			t.Fatalf("getting updated CR: %v", err)
		}

		conditionTypes := map[string]bool{}
		for _, c := range updated.Status.Conditions {
			conditionTypes[c.Type] = true
		}

		for _, ct := range []string{ConditionReady, ConditionAPIServerAvailable, ConditionProcessorAvailable} {
			if !conditionTypes[ct] {
				t.Errorf("missing condition %q", ct)
			}
		}

		if updated.Status.ObservedGeneration != updated.Generation {
			t.Errorf("observedGeneration = %d, want %d", updated.Status.ObservedGeneration, updated.Generation)
		}
	})

	t.Run("updates on spec change", func(t *testing.T) {
		gw := newTestGateway("test-update", "default")
		if err := k8sClient.Create(ctx, gw); err != nil {
			t.Fatalf("creating CR: %v", err)
		}
		t.Cleanup(func() {
			_ = k8sClient.Delete(ctx, gw)
		})

		nn := types.NamespacedName{Name: gw.Name, Namespace: gw.Namespace}

		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		if err != nil {
			t.Fatalf("first Reconcile() error: %v", err)
		}

		if err := k8sClient.Get(ctx, nn, gw); err != nil {
			t.Fatalf("getting CR for update: %v", err)
		}
		gw.Spec.APIServer.Replicas = ptr.To(int32(3))
		if err := k8sClient.Update(ctx, gw); err != nil {
			t.Fatalf("updating CR: %v", err)
		}

		_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		if err != nil {
			t.Fatalf("second Reconcile() error: %v", err)
		}

		var deployList appsv1.DeploymentList
		if err := k8sClient.List(ctx, &deployList); err != nil {
			t.Fatalf("listing deployments: %v", err)
		}

		for _, d := range deployList.Items {
			if !isOwnedByUID(d.OwnerReferences, gw.UID) {
				continue
			}
			component := d.Labels["app.kubernetes.io/component"]
			if component == "apiserver" {
				if d.Spec.Replicas == nil || *d.Spec.Replicas != 3 {
					replicas := int32(0)
					if d.Spec.Replicas != nil {
						replicas = *d.Spec.Replicas
					}
					t.Errorf("apiserver replicas = %d, want 3", replicas)
				}
			}
		}
	})
}

func isOwnedByUID(refs []metav1.OwnerReference, uid types.UID) bool {
	for _, ref := range refs {
		if ref.UID == uid {
			return true
		}
	}
	return false
}
