/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package builder

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	myappv1alpha1 "github.com/example/webapp-operator/api/v1alpha1"
)

func TestComputeConfigHash(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = myappv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "app-config", Namespace: "default"},
		Data:       map[string]string{"KEY1": "val1", "KEY2": "val2"},
	}
	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "db-creds", Namespace: "default"},
		Data:       map[string][]byte{"USER": []byte("admin"), "PASS": []byte("secret")},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm, sec).Build()

	tests := []struct {
		name    string
		webapp  *myappv1alpha1.WebApp
		wantErr bool
		wantNil bool
	}{
		{
			name: "envFrom configMap",
			webapp: &myappv1alpha1.WebApp{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec: myappv1alpha1.WebAppSpec{
					EnvFrom: []myappv1alpha1.EnvFromSource{
						{ConfigMapRef: &myappv1alpha1.ConfigMapEnvSource{Name: "app-config"}},
					},
				},
			},
			wantErr: false,
			wantNil: false,
		},
		{
			name: "envFrom secret",
			webapp: &myappv1alpha1.WebApp{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec: myappv1alpha1.WebAppSpec{
					EnvFrom: []myappv1alpha1.EnvFromSource{
						{SecretRef: &myappv1alpha1.SecretEnvSource{Name: "db-creds"}},
					},
				},
			},
			wantErr: false,
			wantNil: false,
		},
		{
			name: "volume configMap",
			webapp: &myappv1alpha1.WebApp{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec: myappv1alpha1.WebAppSpec{
					Volumes: []myappv1alpha1.VolumeMount{
						{
							Name:      "config",
							MountPath: "/etc/config",
							ConfigMap: &myappv1alpha1.ConfigMapVolumeSource{Name: "app-config"},
						},
					},
				},
			},
			wantErr: false,
			wantNil: false,
		},
		{
			name: "no external config",
			webapp: &myappv1alpha1.WebApp{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec:       myappv1alpha1.WebAppSpec{},
			},
			wantErr: false,
			wantNil: true,
		},
		{
			name: "missing configMap",
			webapp: &myappv1alpha1.WebApp{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec: myappv1alpha1.WebAppSpec{
					EnvFrom: []myappv1alpha1.EnvFromSource{
						{ConfigMapRef: &myappv1alpha1.ConfigMapEnvSource{Name: "missing"}},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := ComputeConfigHash(context.Background(), c, tt.webapp)
			if (err != nil) != tt.wantErr {
				t.Errorf("ComputeConfigHash() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if tt.wantNil && hash != "" {
					t.Errorf("ComputeConfigHash() = %v, want empty", hash)
				}
				if !tt.wantNil && hash == "" {
					t.Errorf("ComputeConfigHash() = empty, want non-empty")
				}
			}
		})
	}
}
