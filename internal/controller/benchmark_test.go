/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package controller

import (
	"context"
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	myappv1alpha1 "github.com/example/webapp-operator/api/v1alpha1"
)

func makeWebApp(name string, ingress bool) *myappv1alpha1.WebApp {
	w := &myappv1alpha1.WebApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: myappv1alpha1.WebAppSpec{
			Image:          "nginx:1.25",
			Replicas:       ptr.To(int32(2)),
			Port:           8080,
			ServiceType:    corev1.ServiceTypeClusterIP,
			UpdateStrategy: "RollingUpdate",
		},
	}
	if ingress {
		w.Spec.EnableIngress = true
		w.Spec.IngressHost = name + ".example.com"
	}
	return w
}

// buildClient constructs a fake client preloaded with a single WebApp.
func buildClient(b *testing.B, webapp *myappv1alpha1.WebApp) *WebAppReconciler {
	b.Helper()
	s := newTestScheme(&testing.T{})
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(webapp).
		WithStatusSubresource(&myappv1alpha1.WebApp{}).
		Build()
	return &WebAppReconciler{
		Client:   c,
		Scheme:   s,
		Recorder: record.NewFakeRecorder(100),
	}
}

// BenchmarkReconcile_FirstPass measures a cold reconcile: finalizer is
// missing, no sub-resources exist. This represents the cost of bringing
// a brand-new WebApp into existence.
//
// Note: this resets the reconciler each iteration so we always measure
// the "first reconcile" cost, not subsequent steady-state passes.
func BenchmarkReconcile_FirstPass(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		b.StopTimer()
		w := makeWebApp("bench", false)
		r := buildClient(b, w)
		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "bench", Namespace: "default"}}
		b.StartTimer()
		_, _ = r.Reconcile(context.Background(), req)
	}
}

// BenchmarkReconcile_SteadyState measures repeated reconciles on the
// same WebApp where the finalizer and phase are already set and
// sub-resources exist — the realistic per-tick cost in production.
func BenchmarkReconcile_SteadyState(b *testing.B) {
	w := makeWebApp("bench", false)
	r := buildClient(b, w)
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "bench", Namespace: "default"}}

	// Prime the state: finalizer + phase + sub-resources
	for i := 0; i < 3; i++ {
		_, _ = r.Reconcile(context.Background(), req)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = r.Reconcile(context.Background(), req)
	}
}

// BenchmarkReconcile_WithIngress measures steady-state reconcile when
// the WebApp also manages an Ingress resource.
func BenchmarkReconcile_WithIngress(b *testing.B) {
	w := makeWebApp("bench", true)
	r := buildClient(b, w)
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "bench", Namespace: "default"}}

	for i := 0; i < 3; i++ {
		_, _ = r.Reconcile(context.Background(), req)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = r.Reconcile(context.Background(), req)
	}
}

// BenchmarkReconcile_Parallel measures parallel reconcile throughput
// across distinct WebApp objects (one per goroutine).
func BenchmarkReconcile_Parallel(b *testing.B) {
	// Pre-create N webapps
	const numApps = 16
	apps := make([]*myappv1alpha1.WebApp, numApps)
	for i := 0; i < numApps; i++ {
		apps[i] = makeWebApp(fmt.Sprintf("bench-%d", i), false)
	}

	s := newTestScheme(&testing.T{})
	cb := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&myappv1alpha1.WebApp{})
	for _, a := range apps {
		cb = cb.WithObjects(a)
	}
	c := cb.Build()
	r := &WebAppReconciler{
		Client:   c,
		Scheme:   s,
		Recorder: record.NewFakeRecorder(1000),
	}

	// Prime each
	for i := 0; i < numApps; i++ {
		req := reconcile.Request{NamespacedName: types.NamespacedName{Name: apps[i].Name, Namespace: "default"}}
		for j := 0; j < 3; j++ {
			_, _ = r.Reconcile(context.Background(), req)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	var counter int64
	b.RunParallel(func(pb *testing.PB) {
		i := int(counter)
		counter++
		for pb.Next() {
			req := reconcile.Request{NamespacedName: types.NamespacedName{
				Name: apps[i%numApps].Name, Namespace: "default",
			}}
			_, _ = r.Reconcile(context.Background(), req)
			i++
		}
	})
}
