// Copyright Red Hat

package helpers

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MergeStatusConditions returns a new status condition array with merged status conditions. It is based on newConditions,
// and merges the corresponding existing conditions if exists.
func MergeStatusConditions(conditions []metav1.Condition, newConditions ...metav1.Condition) []metav1.Condition {
	merged := []metav1.Condition{}

	merged = append(merged, conditions...)

	for _, condition := range newConditions {
		// merge two conditions if necessary
		meta.SetStatusCondition(&merged, condition)
	}

	return merged
}
