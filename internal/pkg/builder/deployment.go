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

package builder

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	myappv1alpha1 "github.com/example/webapp-operator/api/v1alpha1"
	"github.com/example/webapp-operator/internal/pkg/k8sutil"
)

// BuildDeployment constructs the desired Deployment for a WebApp.
// Returns error if resource quantities cannot be parsed.
//
// Important: TypeMeta is set explicitly because Server-Side Apply
// requires the GroupVersionKind to identify the type being applied —
// it is not auto-populated by the typed client.
func BuildDeployment(webapp *myappv1alpha1.WebApp) (*appsv1.Deployment, error) {
	labels := k8sutil.CommonLabels(webapp)

	// When HPA is configured we deliberately leave Deployment.Spec.Replicas
	// nil so the HPA owns this field. If we set it via SSA, every reconcile
	// would fight the HPA (controller sets replicas=spec.replicas → HPA
	// scales to N → controller resets → ...).
	var replicasPtr *int32
	if webapp.Spec.Autoscaling == nil {
		r := webapp.GetReplicas()
		replicasPtr = &r
	}

	deploy := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      webapp.Name,
			Namespace: webapp.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: replicasPtr,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				// Hardened pod/container security context:
				//   - RunAsNonRoot + drop ALL caps + readOnlyRootFilesystem
				//     satisfies the "restricted" Pod Security Standard;
				//   - SeccompProfile RuntimeDefault blocks dangerous syscalls;
				//   - AllowPrivilegeEscalation=false prevents setuid binaries.
				// These are required for clusters that enforce PSS at the
				// namespace level via `pod-security.kubernetes.io/enforce`.
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: ptr.To(true),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "webapp",
							Image: webapp.Spec.Image,
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: webapp.Spec.Port,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: ptr.To(false),
								ReadOnlyRootFilesystem:   ptr.To(true),
								RunAsNonRoot:             ptr.To(true),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
							},
						},
					},
				},
			},
		},
	}

	container := &deploy.Spec.Template.Spec.Containers[0]

	// Set update strategy
	if webapp.Spec.UpdateStrategy == "Recreate" {
		deploy.Spec.Strategy = appsv1.DeploymentStrategy{
			Type: appsv1.RecreateDeploymentStrategyType,
		}
	} else {
		deploy.Spec.Strategy = appsv1.DeploymentStrategy{
			Type: appsv1.RollingUpdateDeploymentStrategyType,
			RollingUpdate: &appsv1.RollingUpdateDeployment{
				MaxUnavailable: intStrPtr(intstr.FromString("25%")),
				MaxSurge:       intStrPtr(intstr.FromString("25%")),
			},
		}
	}

	// Set environment variables
	if len(webapp.Spec.Env) > 0 {
		envVars := make([]corev1.EnvVar, 0, len(webapp.Spec.Env))
		for _, e := range webapp.Spec.Env {
			envVars = append(envVars, corev1.EnvVar{
				Name:  e.Name,
				Value: e.Value,
			})
		}
		container.Env = envVars
	}

	// Set envFrom (ConfigMap/Secret → all keys as env vars)
	if len(webapp.Spec.EnvFrom) > 0 {
		envFrom := make([]corev1.EnvFromSource, 0, len(webapp.Spec.EnvFrom))
		for _, ef := range webapp.Spec.EnvFrom {
			src := corev1.EnvFromSource{}
			if ef.ConfigMapRef != nil {
				src.ConfigMapRef = &corev1.ConfigMapEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: ef.ConfigMapRef.Name},
				}
			}
			if ef.SecretRef != nil {
				src.SecretRef = &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: ef.SecretRef.Name},
				}
			}
			envFrom = append(envFrom, src)
		}
		container.EnvFrom = envFrom
	}

	// Set volumes + volumeMounts
	if len(webapp.Spec.Volumes) > 0 {
		volumes := make([]corev1.Volume, 0, len(webapp.Spec.Volumes))
		mounts := make([]corev1.VolumeMount, 0, len(webapp.Spec.Volumes))

		for _, v := range webapp.Spec.Volumes {
			vol := corev1.Volume{Name: v.Name}
			if v.ConfigMap != nil {
				vol.VolumeSource = corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: v.ConfigMap.Name},
					},
				}
			}
			if v.Secret != nil {
				vol.VolumeSource = corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: v.Secret.Name,
					},
				}
			}
			volumes = append(volumes, vol)
			mounts = append(mounts, corev1.VolumeMount{
				Name:      v.Name,
				MountPath: v.MountPath,
			})
		}

		deploy.Spec.Template.Spec.Volumes = volumes
		container.VolumeMounts = mounts
	}

	// Set resource requirements
	if webapp.Spec.Resources != nil {
		rr, err := buildResourceRequirements(webapp.Spec.Resources)
		if err != nil {
			return nil, fmt.Errorf("invalid resource requirements: %w", err)
		}
		container.Resources = rr
	}

	// Set health checks
	if webapp.Spec.HealthCheck != nil {
		probe := &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: webapp.Spec.HealthCheck.Path,
					Port: intstr.FromInt32(webapp.Spec.Port),
				},
			},
			InitialDelaySeconds: webapp.Spec.HealthCheck.InitialDelaySeconds,
			PeriodSeconds:       webapp.Spec.HealthCheck.PeriodSeconds,
		}
		container.LivenessProbe = probe
		container.ReadinessProbe = probe.DeepCopy()
	}

	return deploy, nil
}

func buildResourceRequirements(res *myappv1alpha1.ResourceRequirements) (corev1.ResourceRequirements, error) {
	rr := corev1.ResourceRequirements{}
	if res.Requests != nil {
		rl, err := parseResourceList(res.Requests)
		if err != nil {
			return rr, fmt.Errorf("invalid requests: %w", err)
		}
		rr.Requests = rl
	}
	if res.Limits != nil {
		rl, err := parseResourceList(res.Limits)
		if err != nil {
			return rr, fmt.Errorf("invalid limits: %w", err)
		}
		rr.Limits = rl
	}
	return rr, nil
}

func parseResourceList(rl *myappv1alpha1.ResourceList) (corev1.ResourceList, error) {
	result := corev1.ResourceList{}
	if rl.CPU != "" {
		q, err := resource.ParseQuantity(rl.CPU)
		if err != nil {
			return nil, fmt.Errorf("invalid cpu %q: %w", rl.CPU, err)
		}
		result[corev1.ResourceCPU] = q
	}
	if rl.Memory != "" {
		q, err := resource.ParseQuantity(rl.Memory)
		if err != nil {
			return nil, fmt.Errorf("invalid memory %q: %w", rl.Memory, err)
		}
		result[corev1.ResourceMemory] = q
	}
	return result, nil
}

func intStrPtr(val intstr.IntOrString) *intstr.IntOrString {
	return &val
}
