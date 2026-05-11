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

package k8sutil

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	myappv1alpha1 "github.com/example/webapp-operator/api/v1alpha1"
)

// SetOwnerReference sets the OwnerReference on the owned object pointing to the WebApp.
func SetOwnerReference(webapp *myappv1alpha1.WebApp, owned metav1.Object, scheme *runtime.Scheme) error {
	if err := controllerutil.SetControllerReference(webapp, owned, scheme); err != nil {
		return fmt.Errorf("failed to set owner reference: %w", err)
	}
	return nil
}

// CommonLabels returns the standard set of labels for resources owned by a WebApp.
func CommonLabels(webapp *myappv1alpha1.WebApp) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "webapp",
		"app.kubernetes.io/instance":   webapp.Name,
		"app.kubernetes.io/managed-by": "webapp-operator",
	}
}
