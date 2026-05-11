/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package v1alpha1_test

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	v1alpha1 "github.com/example/webapp-operator/api/v1alpha1"
	v1beta1 "github.com/example/webapp-operator/api/v1beta1"
)

// fullyPopulatedAlpha returns a WebApp with every field set, so the
// round-trip test catches accidentally dropped fields in the converter.
func fullyPopulatedAlpha() *v1alpha1.WebApp {
	return &v1alpha1.WebApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "round-trip",
			Namespace:  "default",
			Generation: 7,
			Labels:     map[string]string{"app.kubernetes.io/managed-by": "test"},
		},
		Spec: v1alpha1.WebAppSpec{
			Image:    "nginx:1.25",
			Replicas: ptr.To(int32(3)),
			Port:     8080,
			Resources: &v1alpha1.ResourceRequirements{
				Requests: &v1alpha1.ResourceList{CPU: "100m", Memory: "128Mi"},
				Limits:   &v1alpha1.ResourceList{CPU: "500m", Memory: "512Mi"},
			},
			Env: []v1alpha1.EnvVar{
				{Name: "ENV", Value: "prod"},
				{Name: "LOG_LEVEL", Value: "info"},
			},
			ServiceType:   corev1.ServiceTypeClusterIP,
			EnableIngress: true,
			IngressHost:   "round-trip.example.com",
			HealthCheck: &v1alpha1.HealthCheck{
				Path:                "/healthz",
				InitialDelaySeconds: 5,
				PeriodSeconds:       10,
			},
			UpdateStrategy: "RollingUpdate",
			Autoscaling: &v1alpha1.AutoscalingSpec{
				MinReplicas:                       ptr.To(int32(2)),
				MaxReplicas:                       10,
				TargetCPUUtilizationPercentage:    ptr.To(int32(70)),
				TargetMemoryUtilizationPercentage: ptr.To(int32(80)),
			},
		},
		Status: v1alpha1.WebAppStatus{
			Phase:              v1alpha1.PhaseRunning,
			ReadyReplicas:      3,
			DesiredReplicas:    3,
			ServiceName:        "round-trip",
			IngressURL:         "http://round-trip.example.com",
			ObservedGeneration: 7,
			Conditions: []metav1.Condition{
				{
					Type:               "Available",
					Status:             metav1.ConditionTrue,
					Reason:             "Ready",
					Message:            "All replicas ready",
					ObservedGeneration: 7,
				},
			},
		},
	}
}

// TestRoundTrip_AlphaBetaAlpha verifies that converting a fully-populated
// v1alpha1 object to v1beta1 and back yields a bit-identical object.
// Any drift here is a silent data-loss bug in the converter.
func TestRoundTrip_AlphaBetaAlpha(t *testing.T) {
	src := fullyPopulatedAlpha()
	hub := &v1beta1.WebApp{}
	if err := src.ConvertTo(hub); err != nil {
		t.Fatalf("ConvertTo(v1beta1) failed: %v", err)
	}

	got := &v1alpha1.WebApp{}
	if err := got.ConvertFrom(hub); err != nil {
		t.Fatalf("ConvertFrom(v1beta1) failed: %v", err)
	}

	if !reflect.DeepEqual(src, got) {
		t.Errorf("round-trip mismatch\n--- src:\n%+v\n--- got:\n%+v", src, got)
	}
}

// TestConvertTo_NilOptionalFields ensures the converter does not allocate
// empty wrappers for nil optional fields (would otherwise create false
// diffs when comparing converted objects).
func TestConvertTo_NilOptionalFields(t *testing.T) {
	src := &v1alpha1.WebApp{
		ObjectMeta: metav1.ObjectMeta{Name: "minimal", Namespace: "default"},
		Spec:       v1alpha1.WebAppSpec{Image: "nginx"},
	}
	hub := &v1beta1.WebApp{}
	if err := src.ConvertTo(hub); err != nil {
		t.Fatalf("ConvertTo failed: %v", err)
	}

	if hub.Spec.Resources != nil {
		t.Errorf("expected nil Resources, got %+v", hub.Spec.Resources)
	}
	if hub.Spec.HealthCheck != nil {
		t.Errorf("expected nil HealthCheck, got %+v", hub.Spec.HealthCheck)
	}
	if hub.Spec.Env != nil {
		t.Errorf("expected nil Env, got %+v", hub.Spec.Env)
	}
}

// TestConvertTo_ResourcesPartial covers the case where only one of
// requests/limits is set — a common real-world configuration that's easy
// to break with a careless copy.
func TestConvertTo_ResourcesPartial(t *testing.T) {
	src := &v1alpha1.WebApp{
		ObjectMeta: metav1.ObjectMeta{Name: "partial", Namespace: "default"},
		Spec: v1alpha1.WebAppSpec{
			Image: "nginx",
			Resources: &v1alpha1.ResourceRequirements{
				Requests: &v1alpha1.ResourceList{CPU: "100m"},
				// Limits intentionally nil
			},
		},
	}
	hub := &v1beta1.WebApp{}
	if err := src.ConvertTo(hub); err != nil {
		t.Fatalf("ConvertTo failed: %v", err)
	}

	if hub.Spec.Resources == nil {
		t.Fatal("expected non-nil Resources")
	}
	if hub.Spec.Resources.Requests == nil || hub.Spec.Resources.Requests.CPU != "100m" {
		t.Errorf("Requests not preserved: %+v", hub.Spec.Resources.Requests)
	}
	if hub.Spec.Resources.Limits != nil {
		t.Errorf("expected nil Limits, got %+v", hub.Spec.Resources.Limits)
	}
}
