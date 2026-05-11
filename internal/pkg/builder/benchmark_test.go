/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package builder

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	myappv1alpha1 "github.com/example/webapp-operator/api/v1alpha1"
)

func benchWebApp(env, resources, health bool) *myappv1alpha1.WebApp {
	w := &myappv1alpha1.WebApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bench",
			Namespace: "default",
		},
		Spec: myappv1alpha1.WebAppSpec{
			Image:          "nginx:1.25",
			Replicas:       ptr.To(int32(3)),
			Port:           8080,
			ServiceType:    corev1.ServiceTypeClusterIP,
			EnableIngress:  true,
			IngressHost:    "bench.example.com",
			UpdateStrategy: "RollingUpdate",
		},
	}
	if env {
		w.Spec.Env = []myappv1alpha1.EnvVar{
			{Name: "ENV", Value: "prod"},
			{Name: "LOG_LEVEL", Value: "info"},
			{Name: "REGION", Value: "us-west-2"},
		}
	}
	if resources {
		w.Spec.Resources = &myappv1alpha1.ResourceRequirements{
			Requests: &myappv1alpha1.ResourceList{CPU: "100m", Memory: "128Mi"},
			Limits:   &myappv1alpha1.ResourceList{CPU: "500m", Memory: "512Mi"},
		}
	}
	if health {
		w.Spec.HealthCheck = &myappv1alpha1.HealthCheck{
			Path:                "/healthz",
			InitialDelaySeconds: 5,
			PeriodSeconds:       10,
		}
	}
	return w
}

// BenchmarkBuildDeployment_Minimal measures the cost of building a Deployment
// with only required fields (image + replicas + port).
func BenchmarkBuildDeployment_Minimal(b *testing.B) {
	w := benchWebApp(false, false, false)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = BuildDeployment(w)
	}
}

// BenchmarkBuildDeployment_Full measures the cost with env vars,
// resource requirements, and health checks all populated.
func BenchmarkBuildDeployment_Full(b *testing.B) {
	w := benchWebApp(true, true, true)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = BuildDeployment(w)
	}
}

// BenchmarkBuildService isolates the Service builder.
func BenchmarkBuildService(b *testing.B) {
	w := benchWebApp(false, false, false)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = BuildService(w)
	}
}

// BenchmarkBuildIngress isolates the Ingress builder.
func BenchmarkBuildIngress(b *testing.B) {
	w := benchWebApp(false, false, false)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = BuildIngress(w)
	}
}

// BenchmarkBuildAll measures the combined cost of all three builders
// per reconcile (a realistic per-reconcile baseline).
func BenchmarkBuildAll(b *testing.B) {
	w := benchWebApp(true, true, true)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = BuildDeployment(w)
		_ = BuildService(w)
		_ = BuildIngress(w)
	}
}
