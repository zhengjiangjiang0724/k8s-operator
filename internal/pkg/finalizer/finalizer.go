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

package finalizer

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	// WebAppFinalizer is the finalizer name for WebApp resources.
	WebAppFinalizer = "myapp.example.com/webapp-finalizer"
)

// EnsureFinalizer adds the finalizer to the object if it doesn't exist.
// Returns true if the finalizer was added (object was updated).
//
// The finalizer pattern lets us run cleanup before Kubernetes garbage
// collects the CR: setting DeletionTimestamp on a finalized object only
// blocks deletion until every finalizer is removed.
func EnsureFinalizer(ctx context.Context, c client.Client, obj client.Object) (bool, error) {
	if controllerutil.ContainsFinalizer(obj, WebAppFinalizer) {
		return false, nil
	}
	controllerutil.AddFinalizer(obj, WebAppFinalizer)
	if err := c.Update(ctx, obj); err != nil {
		return false, err
	}
	return true, nil
}

// RemoveFinalizer removes the finalizer from the object.
func RemoveFinalizer(ctx context.Context, c client.Client, obj client.Object) error {
	controllerutil.RemoveFinalizer(obj, WebAppFinalizer)
	return c.Update(ctx, obj)
}

// HasFinalizer checks if the object has the WebApp finalizer.
func HasFinalizer(obj client.Object) bool {
	return controllerutil.ContainsFinalizer(obj, WebAppFinalizer)
}
