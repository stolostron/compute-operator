// Copyright Red Hat

package helpers

func ManagedClusterSetNameForWorkspace(workspaceName string) string {
	// For now, workspaces are uniquely identified by their name. This may change.
	return workspaceName
}
