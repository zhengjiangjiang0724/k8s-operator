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
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	myappv1alpha1 "github.com/example/webapp-operator/api/v1alpha1"
)

func newTestWebApp() *myappv1alpha1.WebApp {
	return &myappv1alpha1.WebApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app",
			Namespace: "default",
		},
		Spec: myappv1alpha1.WebAppSpec{
			Image:    "nginx:1.25",
			Replicas: ptr.To(int32(3)),
			Port:     80,
		},
	}
}

func TestBuildDeployment_Basic(t *testing.T) {
	webapp := newTestWebApp()
	deploy, err := BuildDeployment(webapp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if deploy.Name != "test-app" {
		t.Errorf("expected name test-app, got %s", deploy.Name)
	}
	if deploy.Namespace != "default" {
		t.Errorf("expected namespace default, got %s", deploy.Namespace)
	}
	if *deploy.Spec.Replicas != 3 {
		t.Errorf("expected 3 replicas, got %d", *deploy.Spec.Replicas)
	}
	if deploy.Kind != "Deployment" {
		t.Errorf("expected Kind=Deployment, got %s", deploy.Kind)
	}

	container := deploy.Spec.Template.Spec.Containers[0]
	if container.Image != "nginx:1.25" {
		t.Errorf("expected image nginx:1.25, got %s", container.Image)
	}
	if container.Ports[0].ContainerPort != 80 {
		t.Errorf("expected port 80, got %d", container.Ports[0].ContainerPort)
	}

	// Verify SecurityContext
	sc := container.SecurityContext
	if sc == nil {
		t.Fatal("expected SecurityContext to be set")
	}
	if *sc.RunAsNonRoot != true {
		t.Error("expected RunAsNonRoot=true")
	}
	if *sc.AllowPrivilegeEscalation != false {
		t.Error("expected AllowPrivilegeEscalation=false")
	}
	if *sc.ReadOnlyRootFilesystem != true {
		t.Error("expected ReadOnlyRootFilesystem=true")
	}
	if len(sc.Capabilities.Drop) != 1 || sc.Capabilities.Drop[0] != "ALL" {
		t.Error("expected Capabilities.Drop=[ALL]")
	}
}

func TestBuildDeployment_WithEnv(t *testing.T) {
	webapp := newTestWebApp()
	webapp.Spec.Env = []myappv1alpha1.EnvVar{
		{Name: "APP_ENV", Value: "production"},
		{Name: "LOG_LEVEL", Value: "info"},
	}
	deploy, err := BuildDeployment(webapp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	envs := deploy.Spec.Template.Spec.Containers[0].Env
	if len(envs) != 2 {
		t.Fatalf("expected 2 env vars, got %d", len(envs))
	}
	if envs[0].Name != "APP_ENV" || envs[0].Value != "production" {
		t.Errorf("unexpected env[0]: %+v", envs[0])
	}
}

func TestBuildDeployment_WithResources(t *testing.T) {
	webapp := newTestWebApp()
	webapp.Spec.Resources = &myappv1alpha1.ResourceRequirements{
		Requests: &myappv1alpha1.ResourceList{CPU: "100m", Memory: "128Mi"},
		Limits:   &myappv1alpha1.ResourceList{CPU: "500m", Memory: "256Mi"},
	}
	deploy, err := BuildDeployment(webapp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	res := deploy.Spec.Template.Spec.Containers[0].Resources
	if res.Requests.Cpu().String() != "100m" {
		t.Errorf("expected cpu request 100m, got %s", res.Requests.Cpu().String())
	}
	if res.Limits.Memory().String() != "256Mi" {
		t.Errorf("expected memory limit 256Mi, got %s", res.Limits.Memory().String())
	}
}

func TestBuildDeployment_InvalidResource(t *testing.T) {
	webapp := newTestWebApp()
	webapp.Spec.Resources = &myappv1alpha1.ResourceRequirements{
		Requests: &myappv1alpha1.ResourceList{CPU: "not-a-quantity"},
	}
	_, err := BuildDeployment(webapp)
	if err == nil {
		t.Fatal("expected error for invalid resource quantity")
	}
}

func TestBuildDeployment_WithHealthCheck(t *testing.T) {
	webapp := newTestWebApp()
	webapp.Spec.HealthCheck = &myappv1alpha1.HealthCheck{
		Path:                "/health",
		InitialDelaySeconds: 5,
		PeriodSeconds:       10,
	}
	deploy, err := BuildDeployment(webapp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	container := deploy.Spec.Template.Spec.Containers[0]
	if container.LivenessProbe == nil {
		t.Fatal("expected LivenessProbe to be set")
	}
	if container.LivenessProbe.HTTPGet.Path != "/health" {
		t.Errorf("expected probe path /health, got %s", container.LivenessProbe.HTTPGet.Path)
	}
	if container.ReadinessProbe == nil {
		t.Fatal("expected ReadinessProbe to be set")
	}
}

const testUpdateStrategyRecreate = "Recreate"

func TestBuildDeployment_RecreateStrategy(t *testing.T) {
	webapp := newTestWebApp()
	webapp.Spec.UpdateStrategy = testUpdateStrategyRecreate
	deploy, err := BuildDeployment(webapp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deploy.Spec.Strategy.Type != "Recreate" {
		t.Errorf("expected Recreate strategy, got %s", deploy.Spec.Strategy.Type)
	}
}

func TestBuildDeployment_DefaultReplicas(t *testing.T) {
	webapp := newTestWebApp()
	webapp.Spec.Replicas = nil
	deploy, err := BuildDeployment(webapp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *deploy.Spec.Replicas != 1 {
		t.Errorf("expected default 1 replica, got %d", *deploy.Spec.Replicas)
	}
}

func TestBuildService_Basic(t *testing.T) {
	webapp := newTestWebApp()
	webapp.Spec.ServiceType = corev1.ServiceTypeNodePort
	svc := BuildService(webapp)

	if svc.Name != "test-app" {
		t.Errorf("expected name test-app, got %s", svc.Name)
	}
	if svc.Kind != "Service" {
		t.Errorf("expected Kind=Service, got %s", svc.Kind)
	}
	if svc.Spec.Type != corev1.ServiceTypeNodePort {
		t.Errorf("expected NodePort, got %s", svc.Spec.Type)
	}
	if svc.Spec.Ports[0].Port != 80 {
		t.Errorf("expected port 80, got %d", svc.Spec.Ports[0].Port)
	}
}

func TestBuildIngress_Disabled(t *testing.T) {
	webapp := newTestWebApp()
	webapp.Spec.EnableIngress = false
	ingress := BuildIngress(webapp)
	if ingress != nil {
		t.Error("expected nil Ingress when disabled")
	}
}

func TestBuildIngress_Enabled(t *testing.T) {
	webapp := newTestWebApp()
	webapp.Spec.EnableIngress = true
	webapp.Spec.IngressHost = "myapp.example.com"
	ingress := BuildIngress(webapp)

	if ingress == nil {
		t.Fatal("expected Ingress to be created")
	}
	if ingress.Kind != "Ingress" {
		t.Errorf("expected Kind=Ingress, got %s", ingress.Kind)
	}
	if ingress.Spec.Rules[0].Host != "myapp.example.com" {
		t.Errorf("expected host myapp.example.com, got %s", ingress.Spec.Rules[0].Host)
	}
}

func TestBuildDeployment_EnvFrom(t *testing.T) {
	webapp := newTestWebApp()
	webapp.Spec.EnvFrom = []myappv1alpha1.EnvFromSource{
		{ConfigMapRef: &myappv1alpha1.ConfigMapEnvSource{Name: "app-config"}},
		{SecretRef: &myappv1alpha1.SecretEnvSource{Name: "db-creds"}},
	}

	deploy, err := BuildDeployment(webapp)
	if err != nil {
		t.Fatalf("BuildDeployment() error = %v", err)
	}

	container := deploy.Spec.Template.Spec.Containers[0]
	if len(container.EnvFrom) != 2 {
		t.Fatalf("expected 2 envFrom, got %d", len(container.EnvFrom))
	}
	if container.EnvFrom[0].ConfigMapRef.Name != "app-config" {
		t.Errorf("expected configMap app-config, got %s", container.EnvFrom[0].ConfigMapRef.Name)
	}
	if container.EnvFrom[1].SecretRef.Name != "db-creds" {
		t.Errorf("expected secret db-creds, got %s", container.EnvFrom[1].SecretRef.Name)
	}
}

func TestBuildDeployment_Volumes(t *testing.T) {
	webapp := newTestWebApp()
	webapp.Spec.Volumes = []myappv1alpha1.VolumeMount{
		{
			Name:      "config",
			MountPath: "/etc/config",
			ConfigMap: &myappv1alpha1.ConfigMapVolumeSource{Name: "nginx-conf"},
		},
		{
			Name:      "certs",
			MountPath: "/etc/ssl",
			Secret:    &myappv1alpha1.SecretVolumeSource{Name: "tls-secret"},
		},
	}

	deploy, err := BuildDeployment(webapp)
	if err != nil {
		t.Fatalf("BuildDeployment() error = %v", err)
	}

	if len(deploy.Spec.Template.Spec.Volumes) != 2 {
		t.Fatalf("expected 2 volumes, got %d", len(deploy.Spec.Template.Spec.Volumes))
	}
	if deploy.Spec.Template.Spec.Volumes[0].ConfigMap.Name != "nginx-conf" {
		t.Errorf("expected configMap nginx-conf, got %s", deploy.Spec.Template.Spec.Volumes[0].ConfigMap.Name)
	}
	if deploy.Spec.Template.Spec.Volumes[1].Secret.SecretName != "tls-secret" {
		t.Errorf("expected secret tls-secret, got %s", deploy.Spec.Template.Spec.Volumes[1].Secret.SecretName)
	}

	container := deploy.Spec.Template.Spec.Containers[0]
	if len(container.VolumeMounts) != 2 {
		t.Fatalf("expected 2 volumeMounts, got %d", len(container.VolumeMounts))
	}
	if container.VolumeMounts[0].MountPath != "/etc/config" {
		t.Errorf("expected mountPath /etc/config, got %s", container.VolumeMounts[0].MountPath)
	}
}
