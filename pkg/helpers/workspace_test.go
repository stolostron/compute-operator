// Copyright Red Hat

package helpers

import (
	"testing"
)

func TestComputeWorkspaceUniqueness(t *testing.T) {
	workspace1 := "root:jane:doe"
	workspace2 := "root:jane-doe"

	expectedWS1 := "root-jane-doe"
	expectedWS2 := "root-jane--doe"

	ws1 := ComputeWorkspaceName(workspace1)
	ws2 := ComputeWorkspaceName(workspace2)
	if ws1 != expectedWS1 {
		t.Fatalf(`Workspace1 name is not as expected. Expected %s, actual %s`, expectedWS1, ws1)
	}
	if ws2 != expectedWS2 {
		t.Fatalf(`Workspace2 name is not as expected. Expected %s, actual %s`, expectedWS2, ws2)
	}
	if ws1 == ws2 {
		t.Fatalf(`Workspace names collide. Workspace1 %s, Workspace2 %s`, ws1, ws2)
	}
}
