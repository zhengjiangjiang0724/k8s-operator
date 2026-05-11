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
	"context"
	"fmt"
	"regexp"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	myappv1alpha1 "github.com/example/webapp-operator/api/v1alpha1"
	appmetrics "github.com/example/webapp-operator/internal/pkg/metrics"
)

var webapplog = logf.Log.WithName("webapp-webhook")

var envNameRegexp = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// SetupWebAppWebhookWithManager registers the webhook for WebApp in the manager.
func SetupWebAppWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &myappv1alpha1.WebApp{}).
		WithValidator(&WebAppCustomValidator{}).
		WithDefaulter(&WebAppCustomDefaulter{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-myapp-example-com-v1alpha1-webapp,mutating=true,failurePolicy=fail,sideEffects=None,groups=myapp.example.com,resources=webapps,verbs=create;update,versions=v1alpha1,name=mwebapp-v1alpha1.kb.io,admissionReviewVersions=v1

// WebAppCustomDefaulter sets default values on WebApp resources.
//
// Defaulting happens via the mutating admission webhook on every create
// and update. CRD-level defaults (+kubebuilder:default) cover most
// scalar fields, but the webhook is required for cross-field defaults
// (e.g. auto-derived ingressHost) and for injecting management labels.
type WebAppCustomDefaulter struct{}

// Default implements webhook.CustomDefaulter.
func (d *WebAppCustomDefaulter) Default(_ context.Context, obj *myappv1alpha1.WebApp) error {
	start := time.Now()
	defer func() {
		appmetrics.WebhookDuration.WithLabelValues("mutating", "create_update").Observe(time.Since(start).Seconds())
		appmetrics.WebhookRequests.WithLabelValues("mutating", "create_update", "allow").Inc()
	}()
	webapplog.Info("Defaulting for WebApp", "name", obj.GetName())

	// Inject standard labels
	if obj.Labels == nil {
		obj.Labels = make(map[string]string)
	}
	obj.Labels["app.kubernetes.io/managed-by"] = "webapp-operator"
	obj.Labels["app.kubernetes.io/instance"] = obj.Name

	// Default replicas
	if obj.Spec.Replicas == nil {
		defaultReplicas := int32(1)
		obj.Spec.Replicas = &defaultReplicas
	}

	// Default port
	if obj.Spec.Port == 0 {
		obj.Spec.Port = 8080
	}

	// Default serviceType
	if obj.Spec.ServiceType == "" {
		obj.Spec.ServiceType = corev1.ServiceTypeClusterIP
	}

	// Default updateStrategy
	if obj.Spec.UpdateStrategy == "" {
		obj.Spec.UpdateStrategy = "RollingUpdate"
	}

	// Auto-generate ingressHost if Ingress is enabled but host is empty
	if obj.Spec.EnableIngress && obj.Spec.IngressHost == "" {
		obj.Spec.IngressHost = fmt.Sprintf("%s.%s.svc.cluster.local", obj.Name, obj.Namespace)
	}

	// Default healthCheck
	if obj.Spec.HealthCheck == nil {
		obj.Spec.HealthCheck = &myappv1alpha1.HealthCheck{
			Path:                "/healthz",
			InitialDelaySeconds: 10,
			PeriodSeconds:       10,
		}
	}

	return nil
}

// +kubebuilder:webhook:path=/validate-myapp-example-com-v1alpha1-webapp,mutating=false,failurePolicy=fail,sideEffects=None,groups=myapp.example.com,resources=webapps,verbs=create;update,versions=v1alpha1,name=vwebapp-v1alpha1.kb.io,admissionReviewVersions=v1

// WebAppCustomValidator validates WebApp resources.
//
// The validator returns a structured apierrors.Invalid so that
// `kubectl apply` surfaces a clean per-field error list. Plain
// fmt.Errorf would be opaque to the user and break kubectl --dry-run
// validation output.
type WebAppCustomValidator struct{}

// ValidateCreate implements webhook.CustomValidator.
func (v *WebAppCustomValidator) ValidateCreate(_ context.Context, obj *myappv1alpha1.WebApp) (admission.Warnings, error) {
	start := time.Now()
	webapplog.Info("Validating WebApp creation", "name", obj.GetName())
	err := validateWebApp(obj)
	recordValidation("create", start, err)
	return nil, err
}

// ValidateUpdate implements webhook.CustomValidator.
func (v *WebAppCustomValidator) ValidateUpdate(_ context.Context, _, newObj *myappv1alpha1.WebApp) (admission.Warnings, error) {
	start := time.Now()
	webapplog.Info("Validating WebApp update", "name", newObj.GetName())
	err := validateWebApp(newObj)
	recordValidation("update", start, err)
	return nil, err
}

// ValidateDelete implements webhook.CustomValidator.
func (v *WebAppCustomValidator) ValidateDelete(_ context.Context, _ *myappv1alpha1.WebApp) (admission.Warnings, error) {
	start := time.Now()
	recordValidation("delete", start, nil)
	return nil, nil
}

// recordValidation emits the validating-webhook metrics. `err` discriminates
// allow / deny so dashboards can plot the deny rate.
func recordValidation(op string, start time.Time, err error) {
	appmetrics.WebhookDuration.WithLabelValues("validating", op).Observe(time.Since(start).Seconds())
	result := "allow"
	if err != nil {
		result = "deny"
	}
	appmetrics.WebhookRequests.WithLabelValues("validating", op, result).Inc()
}

func validateWebApp(webapp *myappv1alpha1.WebApp) error {
	var allErrs field.ErrorList

	// Validate image
	if webapp.Spec.Image == "" {
		allErrs = append(allErrs, field.Required(
			field.NewPath("spec", "image"),
			"image is required"))
	}

	// Validate replicas
	if webapp.Spec.Replicas != nil {
		if *webapp.Spec.Replicas < 0 || *webapp.Spec.Replicas > 100 {
			allErrs = append(allErrs, field.Invalid(
				field.NewPath("spec", "replicas"),
				*webapp.Spec.Replicas,
				"replicas must be between 0 and 100"))
		}
	}

	// Validate port
	if webapp.Spec.Port < 1 || webapp.Spec.Port > 65535 {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec", "port"),
			webapp.Spec.Port,
			"port must be between 1 and 65535"))
	}

	// Validate env variable names
	for i, env := range webapp.Spec.Env {
		if !envNameRegexp.MatchString(env.Name) {
			allErrs = append(allErrs, field.Invalid(
				field.NewPath("spec", "env").Index(i).Child("name"),
				env.Name,
				"env name must match ^[A-Za-z_][A-Za-z0-9_]*$"))
		}
	}

	// Validate ingressHost is required when enableIngress is true
	if webapp.Spec.EnableIngress && webapp.Spec.IngressHost == "" {
		allErrs = append(allErrs, field.Required(
			field.NewPath("spec", "ingressHost"),
			"ingressHost is required when enableIngress is true"))
	}

	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(
		schema.GroupKind{Group: "myapp.example.com", Kind: "WebApp"},
		webapp.Name, allErrs)
}
