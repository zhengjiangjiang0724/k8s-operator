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

package condition

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// TypeAvailable indicates the WebApp is fully available.
	TypeAvailable = "Available"
	// TypeProgressing indicates the WebApp is being created or updated.
	TypeProgressing = "Progressing"
	// TypeDegraded indicates the WebApp has encountered an error.
	TypeDegraded = "Degraded"
)

// SetCondition sets a condition on the given conditions slice.
//
// meta.SetStatusCondition preserves LastTransitionTime when the status
// has not changed, which is important so consumers can tell when the
// condition actually flipped (vs. when we merely re-asserted it).
func SetCondition(conditions *[]metav1.Condition, condType string, status metav1.ConditionStatus, reason, message string, generation int64) {
	meta.SetStatusCondition(conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: generation,
	})
}

// IsConditionTrue returns true if the condition with the given type is True.
func IsConditionTrue(conditions []metav1.Condition, condType string) bool {
	cond := meta.FindStatusCondition(conditions, condType)
	return cond != nil && cond.Status == metav1.ConditionTrue
}
