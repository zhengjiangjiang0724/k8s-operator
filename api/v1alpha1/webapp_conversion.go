/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package v1alpha1

import (
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	v1beta1 "github.com/example/webapp-operator/api/v1beta1"
)

// ConvertTo converts this WebApp (v1alpha1) to the Hub version (v1beta1).
//
// v1beta1 currently has the same schema as v1alpha1, so the conversion is
// a straight field copy. When v1beta1 grows new fields, populate them here
// with sensible defaults; when v1beta1 drops fields, surface a warning via
// the API Server's conversionReviewVersions response.
func (src *WebApp) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.WebApp)

	dst.ObjectMeta = src.ObjectMeta
	dst.Spec = convertSpecToV1Beta1(src.Spec)
	dst.Status = convertStatusToV1Beta1(src.Status)

	return nil
}

// ConvertFrom converts from the Hub version (v1beta1) to this version (v1alpha1).
func (dst *WebApp) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta1.WebApp)

	dst.ObjectMeta = src.ObjectMeta
	dst.Spec = convertSpecFromV1Beta1(src.Spec)
	dst.Status = convertStatusFromV1Beta1(src.Status)

	return nil
}

func convertSpecToV1Beta1(src WebAppSpec) v1beta1.WebAppSpec {
	dst := v1beta1.WebAppSpec{
		Image:          src.Image,
		Replicas:       src.Replicas,
		Port:           src.Port,
		ServiceType:    src.ServiceType,
		EnableIngress:  src.EnableIngress,
		IngressHost:    src.IngressHost,
		UpdateStrategy: src.UpdateStrategy,
	}
	if src.Resources != nil {
		dst.Resources = &v1beta1.ResourceRequirements{
			Requests: convertResourceListToV1Beta1(src.Resources.Requests),
			Limits:   convertResourceListToV1Beta1(src.Resources.Limits),
		}
	}
	if len(src.Env) > 0 {
		dst.Env = make([]v1beta1.EnvVar, len(src.Env))
		for i, e := range src.Env {
			dst.Env[i] = v1beta1.EnvVar{Name: e.Name, Value: e.Value}
		}
	}

	if len(src.EnvFrom) > 0 {
		dst.EnvFrom = make([]v1beta1.EnvFromSource, len(src.EnvFrom))
		for i, ef := range src.EnvFrom {
			dst.EnvFrom[i] = v1beta1.EnvFromSource{}
			if ef.ConfigMapRef != nil {
				dst.EnvFrom[i].ConfigMapRef = &v1beta1.ConfigMapEnvSource{Name: ef.ConfigMapRef.Name}
			}
			if ef.SecretRef != nil {
				dst.EnvFrom[i].SecretRef = &v1beta1.SecretEnvSource{Name: ef.SecretRef.Name}
			}
		}
	}

	if len(src.Volumes) > 0 {
		dst.Volumes = make([]v1beta1.VolumeMount, len(src.Volumes))
		for i, v := range src.Volumes {
			dst.Volumes[i] = v1beta1.VolumeMount{
				Name:      v.Name,
				MountPath: v.MountPath,
			}
			if v.ConfigMap != nil {
				dst.Volumes[i].ConfigMap = &v1beta1.ConfigMapVolumeSource{Name: v.ConfigMap.Name}
			}
			if v.Secret != nil {
				dst.Volumes[i].Secret = &v1beta1.SecretVolumeSource{Name: v.Secret.Name}
			}
		}
	}
	if src.HealthCheck != nil {
		dst.HealthCheck = &v1beta1.HealthCheck{
			Path:                src.HealthCheck.Path,
			InitialDelaySeconds: src.HealthCheck.InitialDelaySeconds,
			PeriodSeconds:       src.HealthCheck.PeriodSeconds,
		}
	}
	if src.Autoscaling != nil {
		dst.Autoscaling = &v1beta1.AutoscalingSpec{
			MinReplicas:                       src.Autoscaling.MinReplicas,
			MaxReplicas:                       src.Autoscaling.MaxReplicas,
			TargetCPUUtilizationPercentage:    src.Autoscaling.TargetCPUUtilizationPercentage,
			TargetMemoryUtilizationPercentage: src.Autoscaling.TargetMemoryUtilizationPercentage,
		}
	}
	return dst
}

func convertSpecFromV1Beta1(src v1beta1.WebAppSpec) WebAppSpec {
	dst := WebAppSpec{
		Image:          src.Image,
		Replicas:       src.Replicas,
		Port:           src.Port,
		ServiceType:    src.ServiceType,
		EnableIngress:  src.EnableIngress,
		IngressHost:    src.IngressHost,
		UpdateStrategy: src.UpdateStrategy,
	}
	if src.Resources != nil {
		dst.Resources = &ResourceRequirements{
			Requests: convertResourceListFromV1Beta1(src.Resources.Requests),
			Limits:   convertResourceListFromV1Beta1(src.Resources.Limits),
		}
	}
	if len(src.Env) > 0 {
		dst.Env = make([]EnvVar, len(src.Env))
		for i, e := range src.Env {
			dst.Env[i] = EnvVar{Name: e.Name, Value: e.Value}
		}
	}

	if len(src.EnvFrom) > 0 {
		dst.EnvFrom = make([]EnvFromSource, len(src.EnvFrom))
		for i, ef := range src.EnvFrom {
			dst.EnvFrom[i] = EnvFromSource{}
			if ef.ConfigMapRef != nil {
				dst.EnvFrom[i].ConfigMapRef = &ConfigMapEnvSource{Name: ef.ConfigMapRef.Name}
			}
			if ef.SecretRef != nil {
				dst.EnvFrom[i].SecretRef = &SecretEnvSource{Name: ef.SecretRef.Name}
			}
		}
	}

	if len(src.Volumes) > 0 {
		dst.Volumes = make([]VolumeMount, len(src.Volumes))
		for i, v := range src.Volumes {
			dst.Volumes[i] = VolumeMount{
				Name:      v.Name,
				MountPath: v.MountPath,
			}
			if v.ConfigMap != nil {
				dst.Volumes[i].ConfigMap = &ConfigMapVolumeSource{Name: v.ConfigMap.Name}
			}
			if v.Secret != nil {
				dst.Volumes[i].Secret = &SecretVolumeSource{Name: v.Secret.Name}
			}
		}
	}
	if src.HealthCheck != nil {
		dst.HealthCheck = &HealthCheck{
			Path:                src.HealthCheck.Path,
			InitialDelaySeconds: src.HealthCheck.InitialDelaySeconds,
			PeriodSeconds:       src.HealthCheck.PeriodSeconds,
		}
	}
	if src.Autoscaling != nil {
		dst.Autoscaling = &AutoscalingSpec{
			MinReplicas:                       src.Autoscaling.MinReplicas,
			MaxReplicas:                       src.Autoscaling.MaxReplicas,
			TargetCPUUtilizationPercentage:    src.Autoscaling.TargetCPUUtilizationPercentage,
			TargetMemoryUtilizationPercentage: src.Autoscaling.TargetMemoryUtilizationPercentage,
		}
	}
	return dst
}

func convertResourceListToV1Beta1(src *ResourceList) *v1beta1.ResourceList {
	if src == nil {
		return nil
	}
	return &v1beta1.ResourceList{CPU: src.CPU, Memory: src.Memory}
}

func convertResourceListFromV1Beta1(src *v1beta1.ResourceList) *ResourceList {
	if src == nil {
		return nil
	}
	return &ResourceList{CPU: src.CPU, Memory: src.Memory}
}

func convertStatusToV1Beta1(src WebAppStatus) v1beta1.WebAppStatus {
	return v1beta1.WebAppStatus{
		Phase:              v1beta1.WebAppPhase(src.Phase),
		ReadyReplicas:      src.ReadyReplicas,
		DesiredReplicas:    src.DesiredReplicas,
		ServiceName:        src.ServiceName,
		IngressURL:         src.IngressURL,
		ObservedGeneration: src.ObservedGeneration,
		// metav1.Condition is shared across API groups — direct slice
		// alias is safe because the underlying type is identical.
		Conditions: src.Conditions,
	}
}

func convertStatusFromV1Beta1(src v1beta1.WebAppStatus) WebAppStatus {
	return WebAppStatus{
		Phase:              WebAppPhase(src.Phase),
		ReadyReplicas:      src.ReadyReplicas,
		DesiredReplicas:    src.DesiredReplicas,
		ServiceName:        src.ServiceName,
		IngressURL:         src.IngressURL,
		ObservedGeneration: src.ObservedGeneration,
		Conditions:         src.Conditions,
	}
}
