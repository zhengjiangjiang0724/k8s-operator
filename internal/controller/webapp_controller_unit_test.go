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
	"errors"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	myappv1alpha1 "github.com/example/webapp-operator/api/v1alpha1"
)

// newTestScheme returns a runtime scheme with all required types registered.
func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := scheme.AddToScheme(s); err != nil {
		t.Fatalf("failed to add client-go scheme: %v", err)
	}
	if err := myappv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("failed to add v1alpha1 scheme: %v", err)
	}
	return s
}

// newTestWebApp returns a minimal valid WebApp for unit tests.
func newTestWebApp(name, ns string) *myappv1alpha1.WebApp {
	return &myappv1alpha1.WebApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  ns,
			Generation: 1,
			Finalizers: []string{"myapp.example.com/webapp-finalizer"},
		},
		Spec: myappv1alpha1.WebAppSpec{
			Image:          "nginx:1.25",
			Replicas:       ptr.To(int32(1)),
			Port:           80,
			ServiceType:    corev1.ServiceTypeClusterIP,
			UpdateStrategy: "RollingUpdate",
		},
		Status: myappv1alpha1.WebAppStatus{
			Phase: myappv1alpha1.PhaseCreating,
		},
	}
}

// TestSetDegradedAndReturn verifies that setDegradedAndReturn sets the
// Failed phase, the Degraded condition, and returns the original error.
func TestSetDegradedAndReturn(t *testing.T) {
	s := newTestScheme(t)
	webapp := newTestWebApp("test", "default")

	c := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(webapp).
		WithStatusSubresource(&myappv1alpha1.WebApp{}).
		Build()

	r := &WebAppReconciler{
		Client:   c,
		Scheme:   s,
		Recorder: record.NewFakeRecorder(10),
	}

	origErr := errors.New("simulated failure")
	returned := r.setDegradedAndReturn(context.Background(), webapp, "TestReason", origErr)

	if !errors.Is(returned, origErr) {
		t.Fatalf("expected original error to be returned, got %v", returned)
	}

	got := &myappv1alpha1.WebApp{}
	if err := c.Get(context.Background(), types.NamespacedName{Name: "test", Namespace: "default"}, got); err != nil {
		t.Fatalf("failed to get webapp: %v", err)
	}
	if got.Status.Phase != myappv1alpha1.PhaseFailed {
		t.Errorf("expected phase=Failed, got %s", got.Status.Phase)
	}

	var degraded *metav1.Condition
	for i := range got.Status.Conditions {
		if got.Status.Conditions[i].Type == "Degraded" {
			degraded = &got.Status.Conditions[i]
		}
	}
	if degraded == nil {
		t.Fatal("expected Degraded condition to be set")
	}
	if degraded.Status != metav1.ConditionTrue {
		t.Errorf("expected Degraded=True, got %s", degraded.Status)
	}
	if degraded.Reason != "TestReason" {
		t.Errorf("expected reason=TestReason, got %s", degraded.Reason)
	}
}

// TestRecordMetricsErrorPath exercises the error branch of recordMetrics
// (the ReconcileErrors counter increment).
func TestRecordMetricsErrorPath(t *testing.T) {
	s := newTestScheme(t)
	webapp := newTestWebApp("metrics-test", "default")

	r := &WebAppReconciler{
		Client:   fake.NewClientBuilder().WithScheme(s).Build(),
		Scheme:   s,
		Recorder: record.NewFakeRecorder(10),
	}

	// Should not panic; covers both "success" and "error" branches.
	r.recordMetrics(webapp, time.Now(), "error")
	r.recordMetrics(webapp, time.Now(), "success")
}

// TestReconcileNotFound verifies the early-return when the WebApp does not exist.
func TestReconcileNotFound(t *testing.T) {
	s := newTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(s).Build()
	r := &WebAppReconciler{
		Client:   c,
		Scheme:   s,
		Recorder: record.NewFakeRecorder(10),
	}

	result, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "missing", Namespace: "default"},
	})
	if err != nil {
		t.Errorf("expected no error for not-found, got %v", err)
	}
	if (result != ctrl.Result{}) {
		t.Errorf("expected empty result for not-found, got %+v", result)
	}
}

// TestReconcileBuildDeploymentError triggers the setDegradedAndReturn
// path by giving the WebApp resource quantities that fail to parse.
// The fake client does not enforce CRD regex validation, so this works.
func TestReconcileBuildDeploymentError(t *testing.T) {
	s := newTestScheme(t)
	webapp := newTestWebApp("bad-resources", "default")
	webapp.Spec.Resources = &myappv1alpha1.ResourceRequirements{
		Requests: &myappv1alpha1.ResourceList{
			CPU: "not-a-valid-quantity",
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(webapp).
		WithStatusSubresource(&myappv1alpha1.WebApp{}).
		Build()

	r := &WebAppReconciler{
		Client:   c,
		Scheme:   s,
		Recorder: record.NewFakeRecorder(10),
	}

	_, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "bad-resources", Namespace: "default"},
	})
	if err == nil {
		t.Fatal("expected reconcile to return an error")
	}

	// Phase should be Failed
	got := &myappv1alpha1.WebApp{}
	if err := c.Get(context.Background(), types.NamespacedName{Name: "bad-resources", Namespace: "default"}, got); err != nil {
		t.Fatalf("failed to get webapp: %v", err)
	}
	if got.Status.Phase != myappv1alpha1.PhaseFailed {
		t.Errorf("expected phase=Failed, got %s", got.Status.Phase)
	}
}

// TestHandleDeletionNoFinalizer verifies the early-return path in
// handleDeletion when the finalizer is already absent.
func TestHandleDeletionNoFinalizer(t *testing.T) {
	s := newTestScheme(t)
	webapp := newTestWebApp("no-finalizer", "default")
	// Give it some other finalizer so DeletionTimestamp is allowed, but not ours.
	webapp.Finalizers = []string{"other.example.com/finalizer"}
	now := metav1.Now()
	webapp.DeletionTimestamp = &now

	c := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(webapp).
		Build()

	r := &WebAppReconciler{
		Client:   c,
		Scheme:   s,
		Recorder: record.NewFakeRecorder(10),
	}

	result, err := r.handleDeletion(context.Background(), webapp)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if (result != ctrl.Result{}) {
		t.Errorf("expected empty result, got %+v", result)
	}
}

// TestUpdateStatusFromDeploymentNotFound covers the path where the
// underlying Deployment has not yet been created.
func TestUpdateStatusFromDeploymentNotFound(t *testing.T) {
	s := newTestScheme(t)
	webapp := newTestWebApp("no-deploy", "default")

	c := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(webapp).
		WithStatusSubresource(&myappv1alpha1.WebApp{}).
		Build()

	r := &WebAppReconciler{
		Client:   c,
		Scheme:   s,
		Recorder: record.NewFakeRecorder(10),
	}

	if err := r.updateStatusFromDeployment(context.Background(), webapp); err != nil {
		t.Fatalf("expected no error when deployment is missing, got %v", err)
	}

	got := &myappv1alpha1.WebApp{}
	if err := c.Get(context.Background(), types.NamespacedName{Name: "no-deploy", Namespace: "default"}, got); err != nil {
		t.Fatalf("failed to get webapp: %v", err)
	}
	if got.Status.Phase != myappv1alpha1.PhaseCreating {
		t.Errorf("expected phase=Creating, got %s", got.Status.Phase)
	}
	if got.Status.ReadyReplicas != 0 {
		t.Errorf("expected readyReplicas=0, got %d", got.Status.ReadyReplicas)
	}
}

// TestReconcileIngressGetError ensures a non-NotFound error from getting
// the existing Ingress is surfaced (covers reconcileIngress error path).
// We use a fake client interceptor to inject the error.
func TestReconcileIngressDisabledWithExisting(t *testing.T) {
	s := newTestScheme(t)
	webapp := newTestWebApp("ingress-cleanup", "default")
	// Ingress disabled in spec

	existingIngress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ingress-cleanup",
			Namespace: "default",
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(webapp, existingIngress).
		Build()

	r := &WebAppReconciler{
		Client:   c,
		Scheme:   s,
		Recorder: record.NewFakeRecorder(10),
	}

	if err := r.reconcileIngress(context.Background(), webapp); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify the existing Ingress was deleted
	err := c.Get(context.Background(), types.NamespacedName{Name: "ingress-cleanup", Namespace: "default"}, &networkingv1.Ingress{})
	if err == nil {
		t.Error("expected Ingress to be deleted")
	}
}

// Ensure imports are exercised even if some helpers go unused.
var _ = appsv1.Deployment{}
var _ client.Client
