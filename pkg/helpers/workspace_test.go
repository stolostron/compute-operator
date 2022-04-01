// Copyright Red Hat

package helpers

import (
	"testing"
)

func TestManagedClusterSetNameForWorkspace(t *testing.T) {
	workspaceName := "janedoe"
	name := ManagedClusterSetNameForWorkspace(workspaceName)
	if workspaceName != name {
		t.Fatalf(`ManagedClusterSet name is not as expected. Expected %s, actual %s`, workspaceName, name)
	}
}
