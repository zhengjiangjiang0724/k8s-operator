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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	myappv1alpha1 "github.com/example/webapp-operator/api/v1alpha1"
)

var _ = Describe("WebApp Webhook", func() {
	var (
		obj       *myappv1alpha1.WebApp
		oldObj    *myappv1alpha1.WebApp
		validator WebAppCustomValidator
		defaulter WebAppCustomDefaulter
		ctx       context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		obj = &myappv1alpha1.WebApp{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-webapp",
				Namespace: "default",
			},
			Spec: myappv1alpha1.WebAppSpec{
				Image:    "nginx:1.25",
				Replicas: ptr.To(int32(3)),
				Port:     80,
			},
		}
		oldObj = obj.DeepCopy()
		validator = WebAppCustomValidator{}
		defaulter = WebAppCustomDefaulter{}
	})

	Context("Defaulting Webhook", func() {
		It("Should set default replicas when nil", func() {
			obj.Spec.Replicas = nil
			Expect(defaulter.Default(ctx, obj)).To(Succeed())
			Expect(obj.Spec.Replicas).NotTo(BeNil())
			Expect(*obj.Spec.Replicas).To(Equal(int32(1)))
		})

		It("Should set default port when zero", func() {
			obj.Spec.Port = 0
			Expect(defaulter.Default(ctx, obj)).To(Succeed())
			Expect(obj.Spec.Port).To(Equal(int32(8080)))
		})

		It("Should set default serviceType", func() {
			obj.Spec.ServiceType = ""
			Expect(defaulter.Default(ctx, obj)).To(Succeed())
			Expect(obj.Spec.ServiceType).To(Equal(corev1.ServiceTypeClusterIP))
		})

		It("Should set default updateStrategy", func() {
			obj.Spec.UpdateStrategy = ""
			Expect(defaulter.Default(ctx, obj)).To(Succeed())
			Expect(obj.Spec.UpdateStrategy).To(Equal("RollingUpdate"))
		})

		It("Should set default healthCheck", func() {
			obj.Spec.HealthCheck = nil
			Expect(defaulter.Default(ctx, obj)).To(Succeed())
			Expect(obj.Spec.HealthCheck).NotTo(BeNil())
			Expect(obj.Spec.HealthCheck.Path).To(Equal("/healthz"))
			Expect(obj.Spec.HealthCheck.InitialDelaySeconds).To(Equal(int32(10)))
			Expect(obj.Spec.HealthCheck.PeriodSeconds).To(Equal(int32(10)))
		})

		It("Should inject standard labels", func() {
			obj.Labels = nil
			Expect(defaulter.Default(ctx, obj)).To(Succeed())
			Expect(obj.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "webapp-operator"))
			Expect(obj.Labels).To(HaveKeyWithValue("app.kubernetes.io/instance", "test-webapp"))
		})

		It("Should auto-generate ingressHost when enableIngress is true and host is empty", func() {
			obj.Spec.EnableIngress = true
			obj.Spec.IngressHost = ""
			Expect(defaulter.Default(ctx, obj)).To(Succeed())
			Expect(obj.Spec.IngressHost).To(Equal("test-webapp.default.svc.cluster.local"))
		})

		It("Should not override existing values", func() {
			obj.Spec.Replicas = ptr.To(int32(5))
			obj.Spec.Port = 9090
			Expect(defaulter.Default(ctx, obj)).To(Succeed())
			Expect(*obj.Spec.Replicas).To(Equal(int32(5)))
			Expect(obj.Spec.Port).To(Equal(int32(9090)))
		})
	})

	Context("Validating Webhook - Create", func() {
		It("Should accept valid WebApp", func() {
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should reject empty image", func() {
			obj.Spec.Image = ""
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("image"))
		})

		It("Should reject invalid port", func() {
			obj.Spec.Port = 0
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("port"))
		})

		It("Should reject replicas > 100", func() {
			obj.Spec.Replicas = ptr.To(int32(101))
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("replicas"))
		})

		It("Should reject invalid env name", func() {
			obj.Spec.Env = []myappv1alpha1.EnvVar{
				{Name: "123invalid", Value: "val"},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("env"))
		})

		It("Should reject enableIngress without ingressHost", func() {
			obj.Spec.EnableIngress = true
			obj.Spec.IngressHost = ""
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("ingressHost"))
		})

		It("Should accept enableIngress with ingressHost", func() {
			obj.Spec.EnableIngress = true
			obj.Spec.IngressHost = "myapp.example.com"
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("Validating Webhook - Update", func() {
		It("Should accept valid update", func() {
			obj.Spec.Replicas = ptr.To(int32(5))
			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should reject invalid update", func() {
			obj.Spec.Image = ""
			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("Validating Webhook - Delete", func() {
		It("Should always allow deletion", func() {
			_, err := validator.ValidateDelete(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
