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

package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	myappv1alpha1 "github.com/example/webapp-operator/api/v1alpha1"
)

var _ = Describe("WebApp Controller", func() {
	const resourceName = "test-webapp"
	const namespace = "default"

	ctx := context.Background()

	namespacedName := types.NamespacedName{
		Name:      resourceName,
		Namespace: namespace,
	}

	newReconciler := func() *WebAppReconciler {
		return &WebAppReconciler{
			Client:   k8sClient,
			Scheme:   k8sClient.Scheme(),
			Recorder: record.NewFakeRecorder(100),
		}
	}

	Context("When creating a new WebApp", func() {
		BeforeEach(func() {
			webapp := &myappv1alpha1.WebApp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespace,
				},
				Spec: myappv1alpha1.WebAppSpec{
					Image:          "nginx:1.25",
					Replicas:       ptr.To(int32(1)),
					Port:           80,
					ServiceType:    corev1.ServiceTypeClusterIP,
					UpdateStrategy: "RollingUpdate",
				},
			}
			Expect(k8sClient.Create(ctx, webapp)).To(Succeed())
		})

		AfterEach(func() {
			webapp := &myappv1alpha1.WebApp{}
			if err := k8sClient.Get(ctx, namespacedName, webapp); err == nil {
				// Remove finalizer first to allow deletion
				webapp.Finalizers = nil
				_ = k8sClient.Update(ctx, webapp)
				_ = k8sClient.Delete(ctx, webapp)
			}
		})

		It("should add finalizer on first reconcile", func() {
			reconciler := newReconciler()

			// First reconcile: adds finalizer
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue()) //nolint:staticcheck // Requeue deprecated but still used in controller

			// Verify finalizer was added
			webapp := &myappv1alpha1.WebApp{}
			Expect(k8sClient.Get(ctx, namespacedName, webapp)).To(Succeed())
			Expect(webapp.Finalizers).To(ContainElement("myapp.example.com/webapp-finalizer"))
		})

		It("should set Creating phase after finalizer", func() {
			reconciler := newReconciler()

			// First reconcile: adds finalizer
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile: sets Creating phase
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue()) //nolint:staticcheck // Requeue deprecated but still used in controller

			webapp := &myappv1alpha1.WebApp{}
			Expect(k8sClient.Get(ctx, namespacedName, webapp)).To(Succeed())
			Expect(webapp.Status.Phase).To(Equal(myappv1alpha1.PhaseCreating))
		})

		It("should create Deployment and Service after setup", func() {
			reconciler := newReconciler()

			// Run multiple reconcile cycles to get past finalizer + phase setup
			for range 3 {
				_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
				Expect(err).NotTo(HaveOccurred())
			}

			// Verify Deployment was created
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, namespacedName, deploy)).To(Succeed())
			Expect(deploy.Spec.Template.Spec.Containers[0].Image).To(Equal("nginx:1.25"))
			Expect(*deploy.Spec.Replicas).To(Equal(int32(1)))

			// Verify Service was created
			svc := &corev1.Service{}
			Expect(k8sClient.Get(ctx, namespacedName, svc)).To(Succeed())
			Expect(svc.Spec.Ports[0].Port).To(Equal(int32(80)))

			// Verify status was updated
			webapp := &myappv1alpha1.WebApp{}
			Expect(k8sClient.Get(ctx, namespacedName, webapp)).To(Succeed())
			Expect(webapp.Status.ServiceName).To(Equal(resourceName))
			Expect(webapp.Status.ObservedGeneration).To(Equal(webapp.Generation))
		})
	})

	Context("When WebApp is not found", func() {
		It("should not return an error", func() {
			reconciler := newReconciler()
			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "nonexistent", Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
		})
	})

	Context("When deleting a WebApp", func() {
		It("should remove finalizer and allow deletion", func() {
			// Create
			webapp := &myappv1alpha1.WebApp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "delete-test",
					Namespace: namespace,
				},
				Spec: myappv1alpha1.WebAppSpec{
					Image:          "nginx:1.25",
					Replicas:       ptr.To(int32(1)),
					Port:           80,
					UpdateStrategy: "RollingUpdate",
				},
			}
			Expect(k8sClient.Create(ctx, webapp)).To(Succeed())

			reconciler := newReconciler()
			nn := types.NamespacedName{Name: "delete-test", Namespace: namespace}

			// Run reconcile cycles to set up finalizer + resources
			for range 3 {
				_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
				Expect(err).NotTo(HaveOccurred())
			}

			// Delete
			Expect(k8sClient.Delete(ctx, webapp)).To(Succeed())

			// Reconcile should handle deletion
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Verify object is gone
			err = k8sClient.Get(ctx, nn, &myappv1alpha1.WebApp{})
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})
	})

	Context("When Deployment reaches ready state", func() {
		const readyName = "ready-test"
		nn := types.NamespacedName{Name: readyName, Namespace: namespace}

		BeforeEach(func() {
			webapp := &myappv1alpha1.WebApp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      readyName,
					Namespace: namespace,
				},
				Spec: myappv1alpha1.WebAppSpec{
					Image:          "nginx:1.25",
					Replicas:       ptr.To(int32(2)),
					Port:           80,
					ServiceType:    corev1.ServiceTypeClusterIP,
					UpdateStrategy: "RollingUpdate",
				},
			}
			Expect(k8sClient.Create(ctx, webapp)).To(Succeed())
		})

		AfterEach(func() {
			webapp := &myappv1alpha1.WebApp{}
			if err := k8sClient.Get(ctx, nn, webapp); err == nil {
				webapp.Finalizers = nil
				_ = k8sClient.Update(ctx, webapp)
				_ = k8sClient.Delete(ctx, webapp)
			}
			// Clean up sub-resources since envtest has no garbage collector
			_ = k8sClient.Delete(ctx, &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: readyName, Namespace: namespace}})
			_ = k8sClient.Delete(ctx, &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: readyName, Namespace: namespace}})
		})

		It("should set Running phase and conditions when all replicas are ready", func() {
			reconciler := newReconciler()

			// Initial reconcile cycles: finalizer → phase → create resources
			for range 3 {
				_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
				Expect(err).NotTo(HaveOccurred())
			}

			// Simulate the Deployment controller by updating status to fully ready
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, nn, deploy)).To(Succeed())
			deploy.Status.ReadyReplicas = 2
			deploy.Status.Replicas = 2
			deploy.Status.UpdatedReplicas = 2
			deploy.Status.AvailableReplicas = 2
			Expect(k8sClient.Status().Update(ctx, deploy)).To(Succeed())

			// Reconcile again to pick up the new deployment status
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(5 * time.Minute))

			// Verify phase and replica counts
			webapp := &myappv1alpha1.WebApp{}
			Expect(k8sClient.Get(ctx, nn, webapp)).To(Succeed())
			Expect(webapp.Status.Phase).To(Equal(myappv1alpha1.PhaseRunning))
			Expect(webapp.Status.ReadyReplicas).To(Equal(int32(2)))
			Expect(webapp.Status.DesiredReplicas).To(Equal(int32(2)))

			// Verify all three status conditions
			Expect(webapp.Status.Conditions).NotTo(BeEmpty())
			condMap := make(map[string]metav1.ConditionStatus)
			for _, c := range webapp.Status.Conditions {
				condMap[c.Type] = c.Status
			}
			Expect(condMap["Available"]).To(Equal(metav1.ConditionTrue))
			Expect(condMap["Progressing"]).To(Equal(metav1.ConditionFalse))
			Expect(condMap["Degraded"]).To(Equal(metav1.ConditionFalse))
		})

		It("should set Updating phase when deployment is partially updated", func() {
			reconciler := newReconciler()
			for range 3 {
				_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
				Expect(err).NotTo(HaveOccurred())
			}

			// Simulate a rolling update in progress: some pods updated, none ready yet
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, nn, deploy)).To(Succeed())
			deploy.Status.Replicas = 2
			deploy.Status.UpdatedReplicas = 1 // > 0 but < desired (2)
			deploy.Status.ReadyReplicas = 0
			deploy.Status.AvailableReplicas = 0
			Expect(k8sClient.Status().Update(ctx, deploy)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			webapp := &myappv1alpha1.WebApp{}
			Expect(k8sClient.Get(ctx, nn, webapp)).To(Succeed())
			Expect(webapp.Status.Phase).To(Equal(myappv1alpha1.PhaseUpdating))
		})

		It("should set Creating phase when deployment has no updated replicas", func() {
			reconciler := newReconciler()
			for range 3 {
				_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
				Expect(err).NotTo(HaveOccurred())
			}

			// After initial creation, deployment has no ready/updated replicas
			webapp := &myappv1alpha1.WebApp{}
			Expect(k8sClient.Get(ctx, nn, webapp)).To(Succeed())
			Expect(webapp.Status.Phase).To(Equal(myappv1alpha1.PhaseCreating))
		})
	})

	Context("When Ingress is enabled", func() {
		const ingressName = "ingress-test"
		nn := types.NamespacedName{Name: ingressName, Namespace: namespace}

		AfterEach(func() {
			webapp := &myappv1alpha1.WebApp{}
			if err := k8sClient.Get(ctx, nn, webapp); err == nil {
				webapp.Finalizers = nil
				_ = k8sClient.Update(ctx, webapp)
				_ = k8sClient.Delete(ctx, webapp)
			}
			// Manually clean up Ingress since envtest has no garbage collector
			ingress := &networkingv1.Ingress{}
			if err := k8sClient.Get(ctx, nn, ingress); err == nil {
				_ = k8sClient.Delete(ctx, ingress)
			}
		})

		It("should create Ingress and set IngressURL in status", func() {
			webapp := &myappv1alpha1.WebApp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ingressName,
					Namespace: namespace,
				},
				Spec: myappv1alpha1.WebAppSpec{
					Image:          "nginx:1.25",
					Replicas:       ptr.To(int32(1)),
					Port:           80,
					EnableIngress:  true,
					IngressHost:    "test.example.com",
					ServiceType:    corev1.ServiceTypeClusterIP,
					UpdateStrategy: "RollingUpdate",
				},
			}
			Expect(k8sClient.Create(ctx, webapp)).To(Succeed())

			reconciler := newReconciler()
			for range 3 {
				_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
				Expect(err).NotTo(HaveOccurred())
			}

			// Verify Ingress was created with correct host
			ingress := &networkingv1.Ingress{}
			Expect(k8sClient.Get(ctx, nn, ingress)).To(Succeed())
			Expect(ingress.Spec.Rules[0].Host).To(Equal("test.example.com"))

			// Verify status has IngressURL
			wa := &myappv1alpha1.WebApp{}
			Expect(k8sClient.Get(ctx, nn, wa)).To(Succeed())
			Expect(wa.Status.IngressURL).To(Equal("http://test.example.com"))
		})

		It("should delete Ingress when disabled", func() {
			webapp := &myappv1alpha1.WebApp{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ingressName,
					Namespace: namespace,
				},
				Spec: myappv1alpha1.WebAppSpec{
					Image:          "nginx:1.25",
					Replicas:       ptr.To(int32(1)),
					Port:           80,
					EnableIngress:  true,
					IngressHost:    "test.example.com",
					ServiceType:    corev1.ServiceTypeClusterIP,
					UpdateStrategy: "RollingUpdate",
				},
			}
			Expect(k8sClient.Create(ctx, webapp)).To(Succeed())

			reconciler := newReconciler()
			for range 3 {
				_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
				Expect(err).NotTo(HaveOccurred())
			}

			// Verify Ingress exists
			Expect(k8sClient.Get(ctx, nn, &networkingv1.Ingress{})).To(Succeed())

			// Disable ingress by updating the spec
			wa := &myappv1alpha1.WebApp{}
			Expect(k8sClient.Get(ctx, nn, wa)).To(Succeed())
			wa.Spec.EnableIngress = false
			wa.Spec.IngressHost = ""
			Expect(k8sClient.Update(ctx, wa)).To(Succeed())

			// Reconcile to trigger ingress deletion
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Verify Ingress was deleted
			err = k8sClient.Get(ctx, nn, &networkingv1.Ingress{})
			Expect(errors.IsNotFound(err)).To(BeTrue())

			// Verify IngressURL is cleared in status
			Expect(k8sClient.Get(ctx, nn, wa)).To(Succeed())
			Expect(wa.Status.IngressURL).To(BeEmpty())
		})
	})

	Context("When Autoscaling is enabled", func() {
		const hpaName = "hpa-test"
		nn := types.NamespacedName{Name: hpaName, Namespace: namespace}

		AfterEach(func() {
			webapp := &myappv1alpha1.WebApp{}
			if err := k8sClient.Get(ctx, nn, webapp); err == nil {
				webapp.Finalizers = nil
				_ = k8sClient.Update(ctx, webapp)
				_ = k8sClient.Delete(ctx, webapp)
			}
			// Clean up owned HPA (no GC in envtest)
			hpa := &autoscalingv2.HorizontalPodAutoscaler{}
			if err := k8sClient.Get(ctx, nn, hpa); err == nil {
				_ = k8sClient.Delete(ctx, hpa)
			}
		})

		It("should create HPA when Autoscaling is configured", func() {
			webapp := &myappv1alpha1.WebApp{
				ObjectMeta: metav1.ObjectMeta{Name: hpaName, Namespace: namespace},
				Spec: myappv1alpha1.WebAppSpec{
					Image:          "nginx:1.25",
					Replicas:       ptr.To(int32(2)),
					Port:           80,
					ServiceType:    corev1.ServiceTypeClusterIP,
					UpdateStrategy: "RollingUpdate",
					Autoscaling: &myappv1alpha1.AutoscalingSpec{
						MinReplicas:                    ptr.To(int32(2)),
						MaxReplicas:                    8,
						TargetCPUUtilizationPercentage: ptr.To(int32(70)),
					},
				},
			}
			Expect(k8sClient.Create(ctx, webapp)).To(Succeed())

			reconciler := newReconciler()
			for range 3 {
				_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
				Expect(err).NotTo(HaveOccurred())
			}

			By("verifying HPA was created with correct min/max + CPU target")
			hpa := &autoscalingv2.HorizontalPodAutoscaler{}
			Expect(k8sClient.Get(ctx, nn, hpa)).To(Succeed())
			Expect(hpa.Spec.MaxReplicas).To(Equal(int32(8)))
			Expect(hpa.Spec.MinReplicas).NotTo(BeNil())
			Expect(*hpa.Spec.MinReplicas).To(Equal(int32(2)))
			Expect(hpa.Spec.ScaleTargetRef.Name).To(Equal(hpaName))
			Expect(hpa.Spec.Metrics).To(HaveLen(1))

			By("verifying our reconciler does NOT fight HPA over replicas")
			// Simulate HPA scaling the Deployment to 5 replicas.
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, nn, deploy)).To(Succeed())
			scaledUp := int32(5)
			deploy.Spec.Replicas = &scaledUp
			Expect(k8sClient.Update(ctx, deploy)).To(Succeed())

			// Run reconcile — our controller should NOT reset replicas back.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			deploy2 := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, nn, deploy2)).To(Succeed())
			Expect(deploy2.Spec.Replicas).NotTo(BeNil())
			Expect(*deploy2.Spec.Replicas).To(Equal(int32(5)),
				"reconcile must not reset HPA's replica count (Deployment.Spec.Replicas not owned by us)")
		})

		It("should delete HPA when Autoscaling is removed", func() {
			webapp := &myappv1alpha1.WebApp{
				ObjectMeta: metav1.ObjectMeta{Name: hpaName, Namespace: namespace},
				Spec: myappv1alpha1.WebAppSpec{
					Image:          "nginx:1.25",
					Replicas:       ptr.To(int32(2)),
					Port:           80,
					ServiceType:    corev1.ServiceTypeClusterIP,
					UpdateStrategy: "RollingUpdate",
					Autoscaling: &myappv1alpha1.AutoscalingSpec{
						MaxReplicas:                    5,
						TargetCPUUtilizationPercentage: ptr.To(int32(80)),
					},
				},
			}
			Expect(k8sClient.Create(ctx, webapp)).To(Succeed())

			reconciler := newReconciler()
			for range 3 {
				_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
				Expect(err).NotTo(HaveOccurred())
			}

			Expect(k8sClient.Get(ctx, nn, &autoscalingv2.HorizontalPodAutoscaler{})).To(Succeed())

			By("removing Autoscaling from spec")
			wa := &myappv1alpha1.WebApp{}
			Expect(k8sClient.Get(ctx, nn, wa)).To(Succeed())
			wa.Spec.Autoscaling = nil
			Expect(k8sClient.Update(ctx, wa)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			By("verifying HPA was deleted")
			err = k8sClient.Get(ctx, nn, &autoscalingv2.HorizontalPodAutoscaler{})
			Expect(errors.IsNotFound(err)).To(BeTrue())

			By("verifying Deployment.Spec.Replicas is now set again")
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, nn, deploy)).To(Succeed())
			Expect(deploy.Spec.Replicas).NotTo(BeNil(),
				"Replicas should be re-set after HPA removal")
		})
	})

	Context("ConfigMap/Secret change detection", func() {
		const cmName = "webapp-cm-test"
		nn := types.NamespacedName{Name: cmName, Namespace: namespace}

		It("should trigger Pod rollout when ConfigMap data changes", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "app-config", Namespace: namespace},
				Data:       map[string]string{"KEY": "value1"},
			}
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())

			webapp := &myappv1alpha1.WebApp{
				ObjectMeta: metav1.ObjectMeta{Name: cmName, Namespace: namespace},
				Spec: myappv1alpha1.WebAppSpec{
					Image:          "nginx:1.25",
					Replicas:       ptr.To(int32(2)),
					Port:           80,
					ServiceType:    corev1.ServiceTypeClusterIP,
					UpdateStrategy: "RollingUpdate",
					EnvFrom: []myappv1alpha1.EnvFromSource{
						{ConfigMapRef: &myappv1alpha1.ConfigMapEnvSource{Name: "app-config"}},
					},
				},
			}
			Expect(k8sClient.Create(ctx, webapp)).To(Succeed())

			reconciler := newReconciler()
			for range 3 {
				_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
				Expect(err).NotTo(HaveOccurred())
			}

			deploy1 := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, nn, deploy1)).To(Succeed())
			hash1 := deploy1.Spec.Template.Annotations["webapp.example.com/config-hash"]
			Expect(hash1).NotTo(BeEmpty(), "config-hash annotation should be set")

			By("updating ConfigMap data")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-config", Namespace: namespace}, cm)).To(Succeed())
			cm.Data["KEY"] = "value2"
			Expect(k8sClient.Update(ctx, cm)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			deploy2 := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, nn, deploy2)).To(Succeed())
			hash2 := deploy2.Spec.Template.Annotations["webapp.example.com/config-hash"]
			Expect(hash2).NotTo(Equal(hash1), "config-hash should change when ConfigMap data changes")
		})
	})
})
