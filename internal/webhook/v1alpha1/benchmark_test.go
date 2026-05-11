/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package v1alpha1

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	myappv1alpha1 "github.com/example/webapp-operator/api/v1alpha1"
)

func freshWebApp() *myappv1alpha1.WebApp {
	return &myappv1alpha1.WebApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bench",
			Namespace: "default",
		},
		Spec: myappv1alpha1.WebAppSpec{
			Image: "nginx:1.25",
		},
	}
}

func validWebApp() *myappv1alpha1.WebApp {
	return &myappv1alpha1.WebApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bench",
			Namespace: "default",
		},
		Spec: myappv1alpha1.WebAppSpec{
			Image:       "nginx:1.25",
			Replicas:    ptr.To(int32(3)),
			Port:        8080,
			ServiceType: corev1.ServiceTypeClusterIP,
			Env: []myappv1alpha1.EnvVar{
				{Name: "ENV", Value: "prod"},
				{Name: "LOG_LEVEL", Value: "info"},
			},
		},
	}
}

// BenchmarkDefaulter measures the mutating webhook's defaulting cost.
func BenchmarkDefaulter(b *testing.B) {
	d := &WebAppCustomDefaulter{}
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = d.Default(ctx, freshWebApp())
	}
}

// BenchmarkValidateCreate measures the validating webhook's create-time cost.
func BenchmarkValidateCreate(b *testing.B) {
	v := &WebAppCustomValidator{}
	ctx := context.Background()
	w := validWebApp()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = v.ValidateCreate(ctx, w)
	}
}

// BenchmarkValidateUpdate measures the validating webhook's update-time cost.
func BenchmarkValidateUpdate(b *testing.B) {
	v := &WebAppCustomValidator{}
	ctx := context.Background()
	oldW := validWebApp()
	newW := validWebApp()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = v.ValidateUpdate(ctx, oldW, newW)
	}
}
