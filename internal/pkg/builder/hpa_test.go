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
	"k8s.io/utils/ptr"

	myappv1alpha1 "github.com/example/webapp-operator/api/v1alpha1"
)

func TestBuildHPA_Nil(t *testing.T) {
	webapp := newTestWebApp()
	// Autoscaling not set → nil HPA (treated as "delete existing" by reconcile)
	if got := BuildHPA(webapp); got != nil {
		t.Errorf("expected nil HPA when Autoscaling not configured, got %+v", got)
	}
}

func TestBuildHPA_CPUOnly(t *testing.T) {
	webapp := newTestWebApp()
	webapp.Spec.Autoscaling = &myappv1alpha1.AutoscalingSpec{
		MinReplicas:                    ptr.To(int32(2)),
		MaxReplicas:                    10,
		TargetCPUUtilizationPercentage: ptr.To(int32(70)),
	}

	hpa := BuildHPA(webapp)
	if hpa == nil {
		t.Fatal("expected HPA")
	}
	if hpa.Kind != "HorizontalPodAutoscaler" {
		t.Errorf("expected Kind=HorizontalPodAutoscaler, got %q", hpa.Kind)
	}
	if hpa.Spec.ScaleTargetRef.Kind != "Deployment" || hpa.Spec.ScaleTargetRef.Name != webapp.Name {
		t.Errorf("expected scaleTargetRef Deployment/%s, got %+v", webapp.Name, hpa.Spec.ScaleTargetRef)
	}
	if hpa.Spec.MaxReplicas != 10 {
		t.Errorf("expected maxReplicas=10, got %d", hpa.Spec.MaxReplicas)
	}
	if hpa.Spec.MinReplicas == nil || *hpa.Spec.MinReplicas != 2 {
		t.Errorf("expected minReplicas=2, got %v", hpa.Spec.MinReplicas)
	}
	if len(hpa.Spec.Metrics) != 1 {
		t.Fatalf("expected 1 metric (CPU only), got %d", len(hpa.Spec.Metrics))
	}
	cpu := hpa.Spec.Metrics[0]
	if cpu.Resource == nil || cpu.Resource.Name != corev1.ResourceCPU {
		t.Errorf("expected CPU resource metric, got %+v", cpu)
	}
	if cpu.Resource.Target.AverageUtilization == nil || *cpu.Resource.Target.AverageUtilization != 70 {
		t.Errorf("expected CPU target 70%%, got %v", cpu.Resource.Target.AverageUtilization)
	}
}

func TestBuildHPA_CPUAndMemory(t *testing.T) {
	webapp := newTestWebApp()
	webapp.Spec.Autoscaling = &myappv1alpha1.AutoscalingSpec{
		MaxReplicas:                       5,
		TargetCPUUtilizationPercentage:    ptr.To(int32(80)),
		TargetMemoryUtilizationPercentage: ptr.To(int32(75)),
	}

	hpa := BuildHPA(webapp)
	if len(hpa.Spec.Metrics) != 2 {
		t.Fatalf("expected 2 metrics (CPU+Memory), got %d", len(hpa.Spec.Metrics))
	}
}

// TestBuildDeployment_NoReplicasWithAutoscaling verifies the replica
// semantic conflict resolution: when HPA is configured, the controller
// must NOT set Deployment.Spec.Replicas, otherwise every reconcile fights
// the HPA.
func TestBuildDeployment_NoReplicasWithAutoscaling(t *testing.T) {
	webapp := newTestWebApp()
	webapp.Spec.Autoscaling = &myappv1alpha1.AutoscalingSpec{
		MaxReplicas:                    5,
		TargetCPUUtilizationPercentage: ptr.To(int32(80)),
	}

	deploy, err := BuildDeployment(webapp)
	if err != nil {
		t.Fatalf("BuildDeployment failed: %v", err)
	}
	if deploy.Spec.Replicas != nil {
		t.Errorf("expected Replicas=nil when HPA configured (HPA owns this field), got %d", *deploy.Spec.Replicas)
	}
}

// TestBuildDeployment_ReplicasWithoutAutoscaling sanity-checks the
// inverse: without HPA, controller must keep setting Replicas.
func TestBuildDeployment_ReplicasWithoutAutoscaling(t *testing.T) {
	webapp := newTestWebApp()
	// Autoscaling NOT set
	deploy, err := BuildDeployment(webapp)
	if err != nil {
		t.Fatalf("BuildDeployment failed: %v", err)
	}
	if deploy.Spec.Replicas == nil {
		t.Error("expected Replicas to be set when HPA not configured")
	}
}
