/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	myappv1alpha1 "github.com/example/webapp-operator/api/v1alpha1"
	"github.com/example/webapp-operator/internal/pkg/builder"
	"github.com/example/webapp-operator/internal/pkg/condition"
	"github.com/example/webapp-operator/internal/pkg/finalizer"
	"github.com/example/webapp-operator/internal/pkg/k8sutil"
	appmetrics "github.com/example/webapp-operator/internal/pkg/metrics"
)

// WebAppReconciler reconciles a WebApp object.
//
// The reconcile loop is level-triggered and idempotent: every iteration
// re-computes the full desired state and applies it via Server-Side Apply.
// We never read-modify-write the spec of owned resources, which avoids
// conflicts with other controllers/users that may share ownership of fields.
type WebAppReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	// Recorder emits Kubernetes Events so operators see lifecycle progress
	// (kubectl describe webapp <name>) without needing controller logs.
	Recorder record.EventRecorder
	// MaxConcurrentReconciles limits how many WebApps the controller can
	// reconcile in parallel. Tune from metrics: increase when the workqueue
	// depth (controller_runtime_workqueue_depth) is consistently > 0.
	// Setting 0 falls back to the default (3).
	MaxConcurrentReconciles int
}

// +kubebuilder:rbac:groups=myapp.example.com,resources=webapps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=myapp.example.com,resources=webapps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=myapp.example.com,resources=webapps/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Metric label values for success/error outcomes — used across
// reconcile / sub-resource operation metrics.
const (
	resultSuccess = "success"
	resultError   = "error"
)

// requeueInterval is the periodic requeue interval for health-check
// reconciliation. Watches already cover spec/status drift; this periodic
// poll catches edge cases (e.g. external mutation of owned resources that
// somehow bypasses the watch cache, or missed informer events).
const requeueInterval = 5 * time.Minute

// Reconcile performs a full reconciliation for a WebApp resource.
//
// The flow is: Get CR → deletion check → finalizer → phase init →
// sub-resource SSA → status patch → metrics. Each step short-circuits
// on error and is safe to retry; controller-runtime will requeue.
func (r *WebAppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	startTime := time.Now()

	// 1. Get the WebApp CR
	webapp := &myappv1alpha1.WebApp{}
	if err := r.Get(ctx, req.NamespacedName, webapp); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("WebApp resource not found, skipping reconcile")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get WebApp: %w", err)
	}

	// 2. Handle deletion with Finalizer
	if !webapp.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, webapp)
	}

	// 3. Ensure Finalizer is present
	added, err := finalizer.EnsureFinalizer(ctx, r.Client, webapp)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure finalizer: %w", err)
	}
	if added {
		// Finalizer was just added, re-fetch to avoid stale object
		return ctrl.Result{Requeue: true}, nil
	}

	// 4. Set phase to Creating if this is a new resource
	if webapp.Status.Phase == "" {
		return ctrl.Result{Requeue: true}, r.updateStatus(ctx, webapp, func(s *myappv1alpha1.WebAppStatus) {
			s.Phase = myappv1alpha1.PhaseCreating
		})
	}

	// 5. Reconcile sub-resources
	if err := r.reconcileDeployment(ctx, webapp); err != nil {
		r.Recorder.Eventf(webapp, corev1.EventTypeWarning, "ReconcileDeploymentFailed",
			"Failed to reconcile Deployment: %v", err)
		r.recordMetrics(webapp, startTime, resultError)
		return ctrl.Result{}, r.setDegradedAndReturn(ctx, webapp, "ReconcileDeploymentFailed", err)
	}

	if err := r.reconcileService(ctx, webapp); err != nil {
		r.Recorder.Eventf(webapp, corev1.EventTypeWarning, "ReconcileServiceFailed",
			"Failed to reconcile Service: %v", err)
		r.recordMetrics(webapp, startTime, resultError)
		return ctrl.Result{}, r.setDegradedAndReturn(ctx, webapp, "ReconcileServiceFailed", err)
	}

	if err := r.reconcileIngress(ctx, webapp); err != nil {
		r.Recorder.Eventf(webapp, corev1.EventTypeWarning, "ReconcileIngressFailed",
			"Failed to reconcile Ingress: %v", err)
		r.recordMetrics(webapp, startTime, resultError)
		return ctrl.Result{}, r.setDegradedAndReturn(ctx, webapp, "ReconcileIngressFailed", err)
	}

	if err := r.reconcileHPA(ctx, webapp); err != nil {
		r.Recorder.Eventf(webapp, corev1.EventTypeWarning, "ReconcileHPAFailed",
			"Failed to reconcile HPA: %v", err)
		r.recordMetrics(webapp, startTime, resultError)
		return ctrl.Result{}, r.setDegradedAndReturn(ctx, webapp, "ReconcileHPAFailed", err)
	}

	// 6. Update status from Deployment state
	if err := r.updateStatusFromDeployment(ctx, webapp); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	// 7. Record metrics
	r.recordMetrics(webapp, startTime, resultSuccess)

	log.Info("Reconcile completed successfully")
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

// handleDeletion handles the finalizer cleanup when the WebApp is being deleted.
func (r *WebAppReconciler) handleDeletion(ctx context.Context, webapp *myappv1alpha1.WebApp) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if !finalizer.HasFinalizer(webapp) {
		return ctrl.Result{}, nil
	}

	log.Info("Performing cleanup for WebApp deletion")

	// Update phase to Deleting
	_ = r.updateStatus(ctx, webapp, func(s *myappv1alpha1.WebAppStatus) {
		s.Phase = myappv1alpha1.PhaseDeleting
	})

	// Perform cleanup (owned resources are garbage collected via OwnerReference,
	// but we can do additional cleanup here for external resources if needed)
	r.Recorder.Event(webapp, corev1.EventTypeNormal, "CleanupComplete", "External resources cleaned up")

	// Remove finalizer to allow deletion
	if err := finalizer.RemoveFinalizer(ctx, r.Client, webapp); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
	}

	// Drop per-WebApp metric series so deleted objects don't keep emitting
	// stale gauges to Prometheus / VictoriaMetrics scrapes.
	clearMetricsForWebApp(webapp.Name, webapp.Namespace)

	log.Info("Finalizer removed, WebApp will be deleted")
	return ctrl.Result{}, nil
}

const fieldManager = "webapp-operator"

// reconcileDeployment ensures the Deployment matches the desired state using Server-Side Apply.
func (r *WebAppReconciler) reconcileDeployment(ctx context.Context, webapp *myappv1alpha1.WebApp) error {
	desired, err := builder.BuildDeployment(webapp)
	if err != nil {
		return fmt.Errorf("failed to build Deployment: %w", err)
	}

	// Inject config hash annotation to trigger rollout on ConfigMap/Secret change
	hash, err := builder.ComputeConfigHash(ctx, r.Client, webapp)
	if err != nil {
		return fmt.Errorf("failed to compute config hash: %w", err)
	}
	if hash != "" {
		if desired.Spec.Template.Annotations == nil {
			desired.Spec.Template.Annotations = make(map[string]string)
		}
		desired.Spec.Template.Annotations["webapp.example.com/config-hash"] = hash
	}

	if err := k8sutil.SetOwnerReference(webapp, desired, r.Scheme); err != nil {
		return err
	}

	return r.ssaApply(ctx, webapp, desired, "Deployment")
}

// reconcileService ensures the Service matches the desired state using Server-Side Apply.
func (r *WebAppReconciler) reconcileService(ctx context.Context, webapp *myappv1alpha1.WebApp) error {
	desired := builder.BuildService(webapp)
	if err := k8sutil.SetOwnerReference(webapp, desired, r.Scheme); err != nil {
		return err
	}

	return r.ssaApply(ctx, webapp, desired, "Service")
}

// reconcileIngress ensures the Ingress matches the desired state.
func (r *WebAppReconciler) reconcileIngress(ctx context.Context, webapp *myappv1alpha1.WebApp) error {
	desired := builder.BuildIngress(webapp)

	existing := &networkingv1.Ingress{}
	key := client.ObjectKey{Name: webapp.Name, Namespace: webapp.Namespace}
	err := r.Get(ctx, key, existing)

	if desired == nil {
		// Ingress not enabled — delete if it exists
		if err == nil {
			r.Recorder.Eventf(webapp, corev1.EventTypeNormal, "DeletingIngress",
				"Deleting Ingress %s", existing.Name)
			start := time.Now()
			delErr := r.Delete(ctx, existing)
			appmetrics.SubresourceOperationDuration.WithLabelValues("Ingress", "delete").Observe(time.Since(start).Seconds())
			result := resultSuccess
			if delErr != nil {
				result = resultError
			}
			appmetrics.SubresourceOperations.WithLabelValues("Ingress", "delete", result).Inc()
			return delErr
		}
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get Ingress: %w", err)
	}

	if err := k8sutil.SetOwnerReference(webapp, desired, r.Scheme); err != nil {
		return err
	}

	return r.ssaApply(ctx, webapp, desired, "Ingress")
}

// reconcileHPA ensures the HorizontalPodAutoscaler matches the desired state.
// When spec.autoscaling is nil, deletes any existing HPA — mirrors the Ingress
// enable/disable lifecycle. When set, the HPA takes over Deployment.replicas.
func (r *WebAppReconciler) reconcileHPA(ctx context.Context, webapp *myappv1alpha1.WebApp) error {
	desired := builder.BuildHPA(webapp)

	existing := &autoscalingv2.HorizontalPodAutoscaler{}
	key := client.ObjectKey{Name: webapp.Name, Namespace: webapp.Namespace}
	err := r.Get(ctx, key, existing)

	if desired == nil {
		if err == nil {
			r.Recorder.Eventf(webapp, corev1.EventTypeNormal, "DeletingHPA",
				"Deleting HPA %s", existing.Name)
			start := time.Now()
			delErr := r.Delete(ctx, existing)
			appmetrics.SubresourceOperationDuration.WithLabelValues("HPA", "delete").Observe(time.Since(start).Seconds())
			result := resultSuccess
			if delErr != nil {
				result = resultError
			}
			appmetrics.SubresourceOperations.WithLabelValues("HPA", "delete", result).Inc()
			return delErr
		}
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get HPA: %w", err)
	}

	if err := k8sutil.SetOwnerReference(webapp, desired, r.Scheme); err != nil {
		return err
	}

	return r.ssaApply(ctx, webapp, desired, "HorizontalPodAutoscaler")
}

// ssaApply performs a Server-Side Apply using controller-runtime's
// dedicated Apply() method (the older Patch(client.Apply) form was
// deprecated in controller-runtime v0.23).
//
// SSA is preferred over Get-then-Update because it:
//   - is conflict-free for fields owned by this controller (the field
//     manager tracks ownership at the field level, not the object level);
//   - is naturally idempotent — no need to diff desired vs existing;
//   - avoids the lost-update race window between Get and Update.
//
// ForceOwnership lets us take ownership of fields that may have been set
// previously by another manager (e.g. a kubectl apply by a human operator).
//
// Implementation note: we convert our typed builder output to an
// Unstructured first, then wrap it via ApplyConfigurationFromUnstructured.
// The controller-runtime docs warn this loses the "explicit zero vs unset"
// distinction, but our builders always set every field we care about, so
// there is no ambiguity. Migrating to typed apply configurations (e.g.
// appsv1apply.Deployment(...)) would remove this caveat but is a much
// larger refactor.
func (r *WebAppReconciler) ssaApply(ctx context.Context, _ *myappv1alpha1.WebApp, obj client.Object, kind string) error {
	start := time.Now()

	unstrMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		appmetrics.SubresourceOperations.WithLabelValues(kind, "apply", resultError).Inc()
		return fmt.Errorf("failed to convert %s to unstructured: %w", kind, err)
	}
	u := &unstructured.Unstructured{Object: unstrMap}
	ac := client.ApplyConfigurationFromUnstructured(u)

	err = r.Apply(ctx, ac,
		client.FieldOwner(fieldManager),
		client.ForceOwnership,
	)
	appmetrics.SubresourceOperationDuration.WithLabelValues(kind, "apply").Observe(time.Since(start).Seconds())
	if err != nil {
		appmetrics.SubresourceOperations.WithLabelValues(kind, "apply", resultError).Inc()
		return fmt.Errorf("failed to apply %s: %w", kind, err)
	}
	appmetrics.SubresourceOperations.WithLabelValues(kind, "apply", resultSuccess).Inc()
	return nil
}

// updateStatusFromDeployment reads the Deployment state and updates the WebApp status.
func (r *WebAppReconciler) updateStatusFromDeployment(ctx context.Context, webapp *myappv1alpha1.WebApp) error {
	deploy := &appsv1.Deployment{}
	key := client.ObjectKey{Name: webapp.Name, Namespace: webapp.Namespace}
	if err := r.Get(ctx, key, deploy); err != nil {
		if apierrors.IsNotFound(err) {
			return r.updateStatus(ctx, webapp, func(s *myappv1alpha1.WebAppStatus) {
				s.Phase = myappv1alpha1.PhaseCreating
				s.ReadyReplicas = 0
			})
		}
		return fmt.Errorf("failed to get Deployment for status: %w", err)
	}

	return r.updateStatus(ctx, webapp, func(s *myappv1alpha1.WebAppStatus) {
		s.ReadyReplicas = deploy.Status.ReadyReplicas
		s.DesiredReplicas = webapp.GetReplicas()
		s.ServiceName = webapp.Name
		s.ObservedGeneration = webapp.Generation

		if webapp.Spec.EnableIngress && webapp.Spec.IngressHost != "" {
			s.IngressURL = "http://" + webapp.Spec.IngressHost
		} else {
			s.IngressURL = ""
		}

		// Determine phase
		if deploy.Status.ReadyReplicas == webapp.GetReplicas() {
			s.Phase = myappv1alpha1.PhaseRunning
			condition.SetCondition(&s.Conditions, condition.TypeAvailable,
				metav1.ConditionTrue, "DeploymentReady",
				"Deployment has minimum availability", webapp.Generation)
			condition.SetCondition(&s.Conditions, condition.TypeProgressing,
				metav1.ConditionFalse, "DeploymentComplete",
				"Deployment completed", webapp.Generation)
			condition.SetCondition(&s.Conditions, condition.TypeDegraded,
				metav1.ConditionFalse, "NotDegraded",
				"WebApp is running normally", webapp.Generation)
		} else if deploy.Status.UpdatedReplicas > 0 && deploy.Status.UpdatedReplicas < webapp.GetReplicas() {
			s.Phase = myappv1alpha1.PhaseUpdating
			condition.SetCondition(&s.Conditions, condition.TypeProgressing,
				metav1.ConditionTrue, "DeploymentUpdating",
				fmt.Sprintf("Deployment updating: %d/%d replicas ready",
					deploy.Status.ReadyReplicas, webapp.GetReplicas()), webapp.Generation)
		} else {
			s.Phase = myappv1alpha1.PhaseCreating
			condition.SetCondition(&s.Conditions, condition.TypeProgressing,
				metav1.ConditionTrue, "DeploymentProgressing",
				fmt.Sprintf("Deployment progressing: %d/%d replicas ready",
					deploy.Status.ReadyReplicas, webapp.GetReplicas()), webapp.Generation)
		}
	})
}

// updateStatus patches the WebApp status using MergePatch.
//
// We use Patch rather than Update because the status subresource and the
// spec can be modified concurrently (the user mutates spec, we mutate
// status). MergeFrom captures the resourceVersion baseline, so the patch
// is rejected with a Conflict if the object has changed underneath us —
// at which point controller-runtime requeues and we read fresh state.
func (r *WebAppReconciler) updateStatus(
	ctx context.Context,
	webapp *myappv1alpha1.WebApp,
	patchFn func(*myappv1alpha1.WebAppStatus),
) error {
	patch := client.MergeFrom(webapp.DeepCopy())
	patchFn(&webapp.Status)

	return r.Status().Patch(ctx, webapp, patch)
}

// setDegradedAndReturn sets the Degraded condition and returns the original error.
func (r *WebAppReconciler) setDegradedAndReturn(ctx context.Context, webapp *myappv1alpha1.WebApp, reason string, originalErr error) error {
	log := logf.FromContext(ctx)
	if statusErr := r.updateStatus(ctx, webapp, func(s *myappv1alpha1.WebAppStatus) {
		s.Phase = myappv1alpha1.PhaseFailed
		condition.SetCondition(&s.Conditions, condition.TypeDegraded,
			metav1.ConditionTrue, reason, originalErr.Error(), webapp.Generation)
	}); statusErr != nil {
		log.Error(statusErr, "Failed to update degraded status")
	}
	return originalErr
}

// recordMetrics records Prometheus metrics for the reconcile loop.
//
// Called at the end of every reconcile (both happy and error paths) to
// keep counter/gauge values in sync with reality. Per-WebApp gauges use
// {name, namespace} as their identity so labels do not pile up over time.
func (r *WebAppReconciler) recordMetrics(webapp *myappv1alpha1.WebApp, startTime time.Time, result string) {
	duration := time.Since(startTime).Seconds()
	appmetrics.ReconcileDuration.WithLabelValues(webapp.Name, webapp.Namespace, result).Observe(duration)
	appmetrics.ReconcileTotal.WithLabelValues(webapp.Name, webapp.Namespace, result).Inc()

	if result == resultError {
		// Kept for backward compatibility with existing dashboards/alerts.
		appmetrics.ReconcileErrors.WithLabelValues(webapp.Name, webapp.Namespace).Inc()
	}

	// Per-WebApp info gauge — labels carry the descriptive state.
	// We delete and re-set rather than overwriting to handle the case where
	// the image or phase label values change (which would otherwise create
	// orphan series with stale label values).
	appmetrics.WebAppInfo.DeletePartialMatch(prometheus.Labels{
		"name": webapp.Name, "namespace": webapp.Namespace,
	})
	appmetrics.WebAppInfo.WithLabelValues(
		webapp.Name, webapp.Namespace,
		string(webapp.Status.Phase), webapp.Spec.Image,
	).Set(1)
	appmetrics.WebAppReplicasDesired.WithLabelValues(webapp.Name, webapp.Namespace).Set(float64(webapp.GetReplicas()))
	appmetrics.WebAppReplicasReady.WithLabelValues(webapp.Name, webapp.Namespace).Set(float64(webapp.Status.ReadyReplicas))

	// Per-phase indicator: 1 for the current phase, 0 for the rest. This
	// keeps `sum by (phase) (webapp_phase)` accurate without leaking series.
	current := string(webapp.Status.Phase)
	for _, p := range appmetrics.AllPhases {
		v := 0.0
		if p == current {
			v = 1.0
		}
		appmetrics.WebAppPhase.WithLabelValues(webapp.Name, webapp.Namespace, p).Set(v)
	}
}

// clearMetricsForWebApp removes all per-WebApp metric series so a deleted
// resource does not leave behind stale gauges that scrape forever.
func clearMetricsForWebApp(name, namespace string) {
	labels := prometheus.Labels{"name": name, "namespace": namespace}
	appmetrics.WebAppInfo.DeletePartialMatch(labels)
	appmetrics.WebAppReplicasDesired.DeletePartialMatch(labels)
	appmetrics.WebAppReplicasReady.DeletePartialMatch(labels)
	appmetrics.WebAppPhase.DeletePartialMatch(labels)
}

// SetupWithManager wires the controller into the manager.
//
// Owns() registers watches on sub-resources so any drift (manual edits,
// pod restarts, etc.) triggers a reconcile via the OwnerReference. This
// makes the controller level-triggered with respect to owned resources.
//
// MaxConcurrentReconciles allows the controller to process multiple WebApps
// in parallel. Reconciles for the same object are still serialized by
// controller-runtime's work queue, so increasing this is always safe.
func (r *WebAppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	concurrency := r.MaxConcurrentReconciles
	if concurrency <= 0 {
		concurrency = 3
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&myappv1alpha1.WebApp{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&networkingv1.Ingress{}).
		Owns(&autoscalingv2.HorizontalPodAutoscaler{}).
		Watches(&corev1.ConfigMap{}, r.enqueueWebAppsForConfigMap()).
		Watches(&corev1.Secret{}, r.enqueueWebAppsForSecret()).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: concurrency,
		}).
		Named("webapp").
		Complete(r)
}

// enqueueWebAppsForConfigMap returns a handler that enqueues all WebApps
// referencing the changed ConfigMap (via envFrom or volumes).
func (r *WebAppReconciler) enqueueWebAppsForConfigMap() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		cm := obj.(*corev1.ConfigMap)
		return r.findWebAppsReferencingConfigMap(ctx, cm)
	})
}

// enqueueWebAppsForSecret returns a handler that enqueues all WebApps
// referencing the changed Secret (via envFrom or volumes).
func (r *WebAppReconciler) enqueueWebAppsForSecret() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		sec := obj.(*corev1.Secret)
		return r.findWebAppsReferencingSecret(ctx, sec)
	})
}

func (r *WebAppReconciler) findWebAppsReferencingConfigMap(ctx context.Context, cm *corev1.ConfigMap) []reconcile.Request {
	var list myappv1alpha1.WebAppList
	if err := r.List(ctx, &list, client.InNamespace(cm.Namespace)); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, wa := range list.Items {
		if referencesConfigMap(&wa, cm.Name) {
			requests = append(requests, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&wa),
			})
		}
	}
	return requests
}

func (r *WebAppReconciler) findWebAppsReferencingSecret(ctx context.Context, sec *corev1.Secret) []reconcile.Request {
	var list myappv1alpha1.WebAppList
	if err := r.List(ctx, &list, client.InNamespace(sec.Namespace)); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, wa := range list.Items {
		if referencesSecret(&wa, sec.Name) {
			requests = append(requests, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&wa),
			})
		}
	}
	return requests
}

func referencesConfigMap(wa *myappv1alpha1.WebApp, name string) bool {
	for _, ef := range wa.Spec.EnvFrom {
		if ef.ConfigMapRef != nil && ef.ConfigMapRef.Name == name {
			return true
		}
	}
	for _, v := range wa.Spec.Volumes {
		if v.ConfigMap != nil && v.ConfigMap.Name == name {
			return true
		}
	}
	return false
}

func referencesSecret(wa *myappv1alpha1.WebApp, name string) bool {
	for _, ef := range wa.Spec.EnvFrom {
		if ef.SecretRef != nil && ef.SecretRef.Name == name {
			return true
		}
	}
	for _, v := range wa.Spec.Volumes {
		if v.Secret != nil && v.Secret.Name == name {
			return true
		}
	}
	return false
}
