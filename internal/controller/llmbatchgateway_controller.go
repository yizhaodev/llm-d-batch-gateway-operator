package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	batchv1alpha1 "github.com/opendatahub-io/llm-d-batch-gateway-operator/api/v1alpha1"
)

const (
	ConditionReady              = "Ready"
	ConditionAPIServerAvailable = "APIServerAvailable"
	ConditionProcessorAvailable = "ProcessorAvailable"

	fieldOwner = "llmbatchgateway-controller"
)

// +kubebuilder:rbac:groups=batch.llm-d.ai,resources=llmbatchgateways,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch.llm-d.ai,resources=llmbatchgateways/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=batch.llm-d.ai,resources=llmbatchgateways/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services;configmaps;serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get
// +kubebuilder:rbac:groups=cert-manager.io,resources=certificates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors;podmonitors;prometheusrules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

type LLMBatchGatewayReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	HelmRenderer *HelmRenderer
}

func (r *LLMBatchGatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var gw batchv1alpha1.LLMBatchGateway
	if err := r.Get(ctx, req.NamespacedName, &gw); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("fetching LLMBatchGateway: %w", err)
	}

	objects, err := r.HelmRenderer.RenderChart(&gw)
	if err != nil {
		meta.SetStatusCondition(&gw.Status.Conditions, metav1.Condition{
			Type:               ConditionReady,
			Status:             metav1.ConditionFalse,
			Reason:             "RenderFailed",
			Message:            err.Error(),
			ObservedGeneration: gw.Generation,
		})
		if statusErr := r.Status().Update(ctx, &gw); statusErr != nil {
			logger.Error(statusErr, "failed to update status after render failure")
		}
		return ctrl.Result{}, fmt.Errorf("rendering chart: %w", err)
	}

	for _, obj := range objects {
		obj.SetNamespace(gw.Namespace)

		if err := controllerutil.SetControllerReference(&gw, obj, r.Scheme); err != nil {
			return ctrl.Result{}, fmt.Errorf("setting owner reference on %s/%s: %w", obj.GetKind(), obj.GetName(), err)
		}

		if err := r.Patch(ctx, obj, client.Apply, client.FieldOwner(fieldOwner), client.ForceOwnership); err != nil {
			if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
				logger.V(1).Info("skipping resource (CRD not installed)", "kind", obj.GetKind(), "name", obj.GetName())
				continue
			}
			return ctrl.Result{}, fmt.Errorf("applying %s/%s: %w", obj.GetKind(), obj.GetName(), err)
		}
		logger.V(2).Info("applied resource", "kind", obj.GetKind(), "name", obj.GetName())
	}

	if err := r.updateStatus(ctx, &gw); err != nil {
		return ctrl.Result{}, fmt.Errorf("updating status: %w", err)
	}

	return ctrl.Result{}, nil
}

func (r *LLMBatchGatewayReconciler) updateStatus(ctx context.Context, gw *batchv1alpha1.LLMBatchGateway) error {
	var deployments appsv1.DeploymentList
	if err := r.List(ctx, &deployments, client.InNamespace(gw.Namespace), client.MatchingLabels{
		"app.kubernetes.io/instance": gw.Name,
	}); err != nil {
		return fmt.Errorf("listing deployments: %w", err)
	}

	componentStatus := &batchv1alpha1.ComponentStatus{}
	for i := range deployments.Items {
		d := &deployments.Items[i]
		if !isOwnedBy(d, gw) {
			continue
		}

		component, ok := d.Labels["app.kubernetes.io/component"]
		if !ok {
			continue
		}

		status := &batchv1alpha1.ComponentReplicaStatus{
			Replicas:      d.Status.Replicas,
			ReadyReplicas: d.Status.ReadyReplicas,
		}

		switch component {
		case "apiserver":
			componentStatus.APIServer = status
		case "processor":
			componentStatus.Processor = status
		case "gc":
			componentStatus.GC = status
		}
	}

	gw.Status.ComponentStatus = componentStatus
	gw.Status.ObservedGeneration = gw.Generation

	apiAvailable := componentStatus.APIServer != nil && componentStatus.APIServer.ReadyReplicas >= 1
	meta.SetStatusCondition(&gw.Status.Conditions, metav1.Condition{
		Type:               ConditionAPIServerAvailable,
		Status:             conditionStatus(apiAvailable),
		Reason:             conditionReason(apiAvailable, "Available", "Unavailable"),
		ObservedGeneration: gw.Generation,
	})

	procAvailable := componentStatus.Processor != nil && componentStatus.Processor.ReadyReplicas >= 1
	meta.SetStatusCondition(&gw.Status.Conditions, metav1.Condition{
		Type:               ConditionProcessorAvailable,
		Status:             conditionStatus(procAvailable),
		Reason:             conditionReason(procAvailable, "Available", "Unavailable"),
		ObservedGeneration: gw.Generation,
	})

	ready := apiAvailable && procAvailable
	meta.SetStatusCondition(&gw.Status.Conditions, metav1.Condition{
		Type:               ConditionReady,
		Status:             conditionStatus(ready),
		Reason:             conditionReason(ready, "AllComponentsReady", "ComponentsNotReady"),
		ObservedGeneration: gw.Generation,
	})

	return r.Status().Update(ctx, gw)
}

func (r *LLMBatchGatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&batchv1alpha1.LLMBatchGateway{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.ServiceAccount{}).
		Complete(r)
}

func isOwnedBy(obj metav1.Object, owner *batchv1alpha1.LLMBatchGateway) bool {
	for _, ref := range obj.GetOwnerReferences() {
		if ref.UID == owner.UID {
			return true
		}
	}
	return false
}

func conditionStatus(ok bool) metav1.ConditionStatus {
	if ok {
		return metav1.ConditionTrue
	}
	return metav1.ConditionFalse
}

func conditionReason(ok bool, trueReason, falseReason string) string {
	if ok {
		return trueReason
	}
	return falseReason
}

var _ reconcile.Reconciler = (*LLMBatchGatewayReconciler)(nil)
