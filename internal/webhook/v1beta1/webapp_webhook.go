/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Package v1beta1 hosts the admission (defaulter + validator) webhook for
// the v1beta1 WebApp type. The logic mirrors internal/webhook/v1alpha1
// because the two versions currently share the same schema; once v1beta1
// adds new fields, this is where their defaulting/validation lives.
package v1beta1

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

	myappv1beta1 "github.com/example/webapp-operator/api/v1beta1"
	appmetrics "github.com/example/webapp-operator/internal/pkg/metrics"
)

var webapplog = logf.Log.WithName("webapp-v1beta1-webhook")

var envNameRegexp = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// SetupWebAppWebhookWithManager registers the v1beta1 webhook in the manager.
func SetupWebAppWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &myappv1beta1.WebApp{}).
		WithValidator(&WebAppCustomValidator{}).
		WithDefaulter(&WebAppCustomDefaulter{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-myapp-example-com-v1beta1-webapp,mutating=true,failurePolicy=fail,sideEffects=None,groups=myapp.example.com,resources=webapps,verbs=create;update,versions=v1beta1,name=mwebapp-v1beta1.kb.io,admissionReviewVersions=v1

// WebAppCustomDefaulter sets default values on v1beta1 WebApp resources.
type WebAppCustomDefaulter struct{}

func (d *WebAppCustomDefaulter) Default(_ context.Context, obj *myappv1beta1.WebApp) error {
	start := time.Now()
	defer func() {
		appmetrics.WebhookDuration.WithLabelValues("mutating", "create_update").Observe(time.Since(start).Seconds())
		appmetrics.WebhookRequests.WithLabelValues("mutating", "create_update", "allow").Inc()
	}()
	webapplog.Info("Defaulting for WebApp v1beta1", "name", obj.GetName())

	if obj.Labels == nil {
		obj.Labels = make(map[string]string)
	}
	obj.Labels["app.kubernetes.io/managed-by"] = "webapp-operator"
	obj.Labels["app.kubernetes.io/instance"] = obj.Name

	if obj.Spec.Replicas == nil {
		defaultReplicas := int32(1)
		obj.Spec.Replicas = &defaultReplicas
	}
	if obj.Spec.Port == 0 {
		obj.Spec.Port = 8080
	}
	if obj.Spec.ServiceType == "" {
		obj.Spec.ServiceType = corev1.ServiceTypeClusterIP
	}
	if obj.Spec.UpdateStrategy == "" {
		obj.Spec.UpdateStrategy = "RollingUpdate"
	}
	if obj.Spec.EnableIngress && obj.Spec.IngressHost == "" {
		obj.Spec.IngressHost = fmt.Sprintf("%s.%s.svc.cluster.local", obj.Name, obj.Namespace)
	}
	if obj.Spec.HealthCheck == nil {
		obj.Spec.HealthCheck = &myappv1beta1.HealthCheck{
			Path:                "/healthz",
			InitialDelaySeconds: 10,
			PeriodSeconds:       10,
		}
	}
	return nil
}

// +kubebuilder:webhook:path=/validate-myapp-example-com-v1beta1-webapp,mutating=false,failurePolicy=fail,sideEffects=None,groups=myapp.example.com,resources=webapps,verbs=create;update,versions=v1beta1,name=vwebapp-v1beta1.kb.io,admissionReviewVersions=v1

// WebAppCustomValidator validates v1beta1 WebApp resources.
type WebAppCustomValidator struct{}

func (v *WebAppCustomValidator) ValidateCreate(_ context.Context, obj *myappv1beta1.WebApp) (admission.Warnings, error) {
	start := time.Now()
	webapplog.Info("Validating WebApp v1beta1 creation", "name", obj.GetName())
	err := validateWebApp(obj)
	recordValidation("create", start, err)
	return nil, err
}

func (v *WebAppCustomValidator) ValidateUpdate(_ context.Context, _, newObj *myappv1beta1.WebApp) (admission.Warnings, error) {
	start := time.Now()
	webapplog.Info("Validating WebApp v1beta1 update", "name", newObj.GetName())
	err := validateWebApp(newObj)
	recordValidation("update", start, err)
	return nil, err
}

func (v *WebAppCustomValidator) ValidateDelete(_ context.Context, _ *myappv1beta1.WebApp) (admission.Warnings, error) {
	start := time.Now()
	recordValidation("delete", start, nil)
	return nil, nil
}

func recordValidation(op string, start time.Time, err error) {
	appmetrics.WebhookDuration.WithLabelValues("validating", op).Observe(time.Since(start).Seconds())
	result := "allow"
	if err != nil {
		result = "deny"
	}
	appmetrics.WebhookRequests.WithLabelValues("validating", op, result).Inc()
}

func validateWebApp(webapp *myappv1beta1.WebApp) error {
	var allErrs field.ErrorList

	if webapp.Spec.Image == "" {
		allErrs = append(allErrs, field.Required(
			field.NewPath("spec", "image"), "image is required"))
	}

	if webapp.Spec.Replicas != nil {
		if *webapp.Spec.Replicas < 0 || *webapp.Spec.Replicas > 100 {
			allErrs = append(allErrs, field.Invalid(
				field.NewPath("spec", "replicas"),
				*webapp.Spec.Replicas,
				"replicas must be between 0 and 100"))
		}
	}

	if webapp.Spec.Port < 1 || webapp.Spec.Port > 65535 {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec", "port"),
			webapp.Spec.Port,
			"port must be between 1 and 65535"))
	}

	for i, env := range webapp.Spec.Env {
		if !envNameRegexp.MatchString(env.Name) {
			allErrs = append(allErrs, field.Invalid(
				field.NewPath("spec", "env").Index(i).Child("name"),
				env.Name,
				"env name must match ^[A-Za-z_][A-Za-z0-9_]*$"))
		}
	}

	if webapp.Spec.EnableIngress && webapp.Spec.IngressHost == "" {
		allErrs = append(allErrs, field.Required(
			field.NewPath("spec", "ingressHost"),
			"ingressHost is required when enableIngress is true"))
	}

	if a := webapp.Spec.Autoscaling; a != nil {
		autoPath := field.NewPath("spec", "autoscaling")
		minR := int32(1)
		if a.MinReplicas != nil {
			minR = *a.MinReplicas
		}
		if a.MaxReplicas < minR {
			allErrs = append(allErrs, field.Invalid(
				autoPath.Child("maxReplicas"),
				a.MaxReplicas,
				fmt.Sprintf("maxReplicas (%d) must be >= minReplicas (%d)", a.MaxReplicas, minR)))
		}
		if a.TargetCPUUtilizationPercentage == nil && a.TargetMemoryUtilizationPercentage == nil {
			allErrs = append(allErrs, field.Required(
				autoPath,
				"at least one of targetCPUUtilizationPercentage or targetMemoryUtilizationPercentage must be set"))
		}
	}

	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(
		schema.GroupKind{Group: "myapp.example.com", Kind: "WebApp"},
		webapp.Name, allErrs)
}
