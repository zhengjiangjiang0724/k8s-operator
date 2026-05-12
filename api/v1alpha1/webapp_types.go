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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WebAppPhase represents the lifecycle phase of a WebApp resource.
// +kubebuilder:validation:Enum=Pending;Creating;Running;Updating;Failed;Deleting
type WebAppPhase string

const (
	PhasePending  WebAppPhase = "Pending"
	PhaseCreating WebAppPhase = "Creating"
	PhaseRunning  WebAppPhase = "Running"
	PhaseUpdating WebAppPhase = "Updating"
	PhaseFailed   WebAppPhase = "Failed"
	PhaseDeleting WebAppPhase = "Deleting"
)

// ResourceRequirements defines CPU and memory resource requests/limits.
type ResourceRequirements struct {
	// requests specifies the minimum resources required.
	// +optional
	Requests *ResourceList `json:"requests,omitempty"`

	// limits specifies the maximum resources allowed.
	// +optional
	Limits *ResourceList `json:"limits,omitempty"`
}

// ResourceList defines CPU and memory quantities.
type ResourceList struct {
	// cpu specifies the CPU resource quantity (e.g. "100m", "1").
	// +kubebuilder:validation:Pattern=`^[0-9]+(\.[0-9]+)?(m|)$`
	// +optional
	CPU string `json:"cpu,omitempty"`

	// memory specifies the memory resource quantity (e.g. "128Mi", "1Gi").
	// +kubebuilder:validation:Pattern=`^[0-9]+(\.[0-9]+)?(Ki|Mi|Gi|Ti|Pi|Ei|k|M|G|T|P|E)$`
	// +optional
	Memory string `json:"memory,omitempty"`
}

// EnvVar represents an environment variable to set in the container.
type EnvVar struct {
	// name is the environment variable name.
	// +kubebuilder:validation:Pattern=`^[A-Za-z_][A-Za-z0-9_]*$`
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// value is the environment variable value.
	// +optional
	Value string `json:"value,omitempty"`
}

// EnvFromSource represents a source to populate environment variables from.
type EnvFromSource struct {
	// configMapRef references a ConfigMap to populate env vars from all keys.
	// +optional
	ConfigMapRef *ConfigMapEnvSource `json:"configMapRef,omitempty"`

	// secretRef references a Secret to populate env vars from all keys.
	// +optional
	SecretRef *SecretEnvSource `json:"secretRef,omitempty"`
}

// ConfigMapEnvSource selects a ConfigMap to populate env vars.
type ConfigMapEnvSource struct {
	// name is the ConfigMap name in the same namespace.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// SecretEnvSource selects a Secret to populate env vars.
type SecretEnvSource struct {
	// name is the Secret name in the same namespace.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// VolumeMount defines a volume to mount into the container.
type VolumeMount struct {
	// name is a unique identifier for this volume mount.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// mountPath is the path inside the container where the volume is mounted.
	// +kubebuilder:validation:MinLength=1
	MountPath string `json:"mountPath"`

	// configMap references a ConfigMap to mount as a volume.
	// Mutually exclusive with secret.
	// +optional
	ConfigMap *ConfigMapVolumeSource `json:"configMap,omitempty"`

	// secret references a Secret to mount as a volume.
	// Mutually exclusive with configMap.
	// +optional
	Secret *SecretVolumeSource `json:"secret,omitempty"`
}

// ConfigMapVolumeSource references a ConfigMap for volume mounting.
type ConfigMapVolumeSource struct {
	// name is the ConfigMap name in the same namespace.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// SecretVolumeSource references a Secret for volume mounting.
type SecretVolumeSource struct {
	// name is the Secret name in the same namespace.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// HealthCheck defines the health check configuration.
type HealthCheck struct {
	// path is the HTTP path for the health check probe.
	// +kubebuilder:default="/healthz"
	// +optional
	Path string `json:"path,omitempty"`

	// initialDelaySeconds is the number of seconds after the container starts
	// before the probe is initiated.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=10
	// +optional
	InitialDelaySeconds int32 `json:"initialDelaySeconds,omitempty"`

	// periodSeconds is how often (in seconds) to perform the probe.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=10
	// +optional
	PeriodSeconds int32 `json:"periodSeconds,omitempty"`
}

// WebAppSpec defines the desired state of WebApp.
type WebAppSpec struct {
	// image is the container image for the web application.
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`

	// replicas is the number of desired pod replicas.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:default=1
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// port is the container port to expose.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +kubebuilder:default=8080
	// +optional
	Port int32 `json:"port,omitempty"`

	// resources defines CPU and memory resource requirements.
	// +optional
	Resources *ResourceRequirements `json:"resources,omitempty"`

	// env is a list of environment variables to set in the container.
	// +optional
	Env []EnvVar `json:"env,omitempty"`

	// envFrom populates environment variables from ConfigMap or Secret.
	// All keys from the referenced resource become env vars.
	// +optional
	EnvFrom []EnvFromSource `json:"envFrom,omitempty"`

	// volumes mounts ConfigMaps or Secrets as files into the container.
	// +optional
	Volumes []VolumeMount `json:"volumes,omitempty"`

	// serviceType specifies the Kubernetes Service type.
	// +kubebuilder:validation:Enum=ClusterIP;NodePort;LoadBalancer
	// +kubebuilder:default=ClusterIP
	// +optional
	ServiceType corev1.ServiceType `json:"serviceType,omitempty"`

	// enableIngress controls whether an Ingress resource is created.
	// +kubebuilder:default=false
	// +optional
	EnableIngress bool `json:"enableIngress,omitempty"`

	// ingressHost is the hostname for the Ingress resource.
	// Required when enableIngress is true.
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([a-z0-9\-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9\-]*[a-z0-9])?)*$`
	// +optional
	IngressHost string `json:"ingressHost,omitempty"`

	// healthCheck configures the liveness and readiness probes.
	// +optional
	HealthCheck *HealthCheck `json:"healthCheck,omitempty"`

	// updateStrategy controls the deployment update strategy.
	// +kubebuilder:validation:Enum=RollingUpdate;Recreate
	// +kubebuilder:default=RollingUpdate
	// +optional
	UpdateStrategy string `json:"updateStrategy,omitempty"`

	// autoscaling enables HorizontalPodAutoscaler-driven scaling. When set,
	// `spec.replicas` becomes the *initial* replica count and HPA owns
	// subsequent scaling decisions. Leave nil to use static `spec.replicas`.
	// +optional
	Autoscaling *AutoscalingSpec `json:"autoscaling,omitempty"`
}

// AutoscalingSpec configures the HorizontalPodAutoscaler created for this WebApp.
//
// At least one of `targetCPUUtilizationPercentage` or
// `targetMemoryUtilizationPercentage` must be set — HPA needs at least
// one metric to make scaling decisions.
type AutoscalingSpec struct {
	// minReplicas is the lower bound for HPA. Defaults to 1.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=1
	// +optional
	MinReplicas *int32 `json:"minReplicas,omitempty"`

	// maxReplicas is the upper bound for HPA. Must be >= minReplicas.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=1000
	MaxReplicas int32 `json:"maxReplicas"`

	// targetCPUUtilizationPercentage is the target average CPU utilization
	// across pods, expressed as a percentage of pod's CPU request.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	// +optional
	TargetCPUUtilizationPercentage *int32 `json:"targetCPUUtilizationPercentage,omitempty"`

	// targetMemoryUtilizationPercentage is the target average memory
	// utilization across pods, expressed as a percentage of pod's memory request.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	// +optional
	TargetMemoryUtilizationPercentage *int32 `json:"targetMemoryUtilizationPercentage,omitempty"`
}

// WebAppStatus defines the observed state of WebApp.
type WebAppStatus struct {
	// phase represents the current lifecycle phase of the WebApp.
	// +optional
	Phase WebAppPhase `json:"phase,omitempty"`

	// readyReplicas is the number of pods with ready status.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// desiredReplicas is the desired number of pod replicas.
	// +optional
	DesiredReplicas int32 `json:"desiredReplicas,omitempty"`

	// serviceName is the name of the created Service.
	// +optional
	ServiceName string `json:"serviceName,omitempty"`

	// ingressURL is the Ingress URL if Ingress is enabled.
	// +optional
	IngressURL string `json:"ingressURL,omitempty"`

	// observedGeneration is the most recent generation observed.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// conditions represent the current state of the WebApp resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=wa,categories=myapp
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`,description="Current phase"
// +kubebuilder:printcolumn:name="Replicas",type=integer,JSONPath=`.status.readyReplicas`,description="Ready replicas"
// +kubebuilder:printcolumn:name="Service",type=string,JSONPath=`.status.serviceName`,description="Service name"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// WebApp is the Schema for the webapps API.
type WebApp struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of WebApp.
	// +required
	Spec WebAppSpec `json:"spec"`

	// status defines the observed state of WebApp.
	// +optional
	Status WebAppStatus `json:"status,omitzero"`
}

// GetReplicas returns the desired replica count, defaulting to 1.
func (w *WebApp) GetReplicas() int32 {
	if w.Spec.Replicas != nil {
		return *w.Spec.Replicas
	}
	return 1
}

// +kubebuilder:object:root=true

// WebAppList contains a list of WebApp.
type WebAppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []WebApp `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WebApp{}, &WebAppList{})
}
