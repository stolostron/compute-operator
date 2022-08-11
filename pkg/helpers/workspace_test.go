// Copyright Red Hat

package helpers

import (
	"testing"
)

func TestComputeWorkspaceName(t *testing.T) {
	workspaceName := "janedoe"
	name := ComputeWorkspaceName(workspaceName)
	if workspaceName != name {
		t.Fatalf(`Workspace name is not as expected. Expected %s, actual %s`, workspaceName, name)
	}
}
