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
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	myappv1alpha1 "github.com/example/webapp-operator/api/v1alpha1"
	"github.com/example/webapp-operator/internal/pkg/k8sutil"
)

// BuildIngress constructs the desired Ingress for a WebApp.
// Returns nil if Ingress is not enabled — the caller treats nil as
// "delete the existing Ingress if any", so we never need a separate
// "should this exist?" flag in the reconcile loop.
func BuildIngress(webapp *myappv1alpha1.WebApp) *networkingv1.Ingress {
	if !webapp.Spec.EnableIngress {
		return nil
	}

	labels := k8sutil.CommonLabels(webapp)
	pathType := networkingv1.PathTypePrefix

	return &networkingv1.Ingress{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "networking.k8s.io/v1",
			Kind:       "Ingress",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      webapp.Name,
			Namespace: webapp.Namespace,
			Labels:    labels,
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					Host: webapp.Spec.IngressHost,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &pathType,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: webapp.Name,
											Port: networkingv1.ServiceBackendPort{
												Number: webapp.Spec.Port,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}
