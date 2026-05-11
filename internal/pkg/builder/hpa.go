/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package builder

import (
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	myappv1alpha1 "github.com/example/webapp-operator/api/v1alpha1"
	"github.com/example/webapp-operator/internal/pkg/k8sutil"
)

// BuildHPA constructs the desired HorizontalPodAutoscaler for a WebApp.
// Returns nil if Autoscaling is not configured — the caller treats nil as
// "delete the existing HPA if any", mirroring the Ingress pattern.
//
// HPA targets the Deployment owned by the same WebApp (matching name and
// namespace, since builder.BuildDeployment uses the same naming).
func BuildHPA(webapp *myappv1alpha1.WebApp) *autoscalingv2.HorizontalPodAutoscaler {
	a := webapp.Spec.Autoscaling
	if a == nil {
		return nil
	}

	labels := k8sutil.CommonLabels(webapp)

	// Build metrics — both CPU and Memory are optional but at least one
	// must be set (enforced by the validating webhook).
	var metrics []autoscalingv2.MetricSpec
	if a.TargetCPUUtilizationPercentage != nil {
		metrics = append(metrics, autoscalingv2.MetricSpec{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricSource{
				Name: corev1.ResourceCPU,
				Target: autoscalingv2.MetricTarget{
					Type:               autoscalingv2.UtilizationMetricType,
					AverageUtilization: a.TargetCPUUtilizationPercentage,
				},
			},
		})
	}
	if a.TargetMemoryUtilizationPercentage != nil {
		metrics = append(metrics, autoscalingv2.MetricSpec{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricSource{
				Name: corev1.ResourceMemory,
				Target: autoscalingv2.MetricTarget{
					Type:               autoscalingv2.UtilizationMetricType,
					AverageUtilization: a.TargetMemoryUtilizationPercentage,
				},
			},
		})
	}

	return &autoscalingv2.HorizontalPodAutoscaler{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "autoscaling/v2",
			Kind:       "HorizontalPodAutoscaler",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      webapp.Name,
			Namespace: webapp.Namespace,
			Labels:    labels,
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       webapp.Name,
			},
			MinReplicas: a.MinReplicas,
			MaxReplicas: a.MaxReplicas,
			Metrics:     metrics,
		},
	}
}
