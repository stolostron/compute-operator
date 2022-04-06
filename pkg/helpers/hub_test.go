// Copyright Red Hat

package helpers

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetConditionStatusFound(t *testing.T) {
	conditions := []metav1.Condition{
		{
			Type:   "Joined",
			Status: metav1.ConditionTrue,
		},
	}
	status, ok := GetConditionStatus(conditions, "Joined")
	if !ok {
		t.Fatalf("Condition not found as expected.")
	}
	if status != metav1.ConditionTrue {
		t.Fatalf(`Condition status not as expected. Expected %s, actual %s`, metav1.ConditionTrue, status)
	}
}

func TestGetConditionStatusNotFound(t *testing.T) {
	conditions := []metav1.Condition{
		{
			Type:   "Joined",
			Status: metav1.ConditionTrue,
		},
	}
	_, ok := GetConditionStatus(conditions, "Available")
	if ok {
		t.Fatalf("Condition found but expected to be not found.")
	}
}
