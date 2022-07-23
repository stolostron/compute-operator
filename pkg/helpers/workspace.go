// Copyright Red Hat

package helpers

import (
	"fmt"
	"strings"
)

func ManagedClusterSetNameForWorkspace(workspaceName string) string {
	// TODO: THIS IS NOT SUFFICIENT AT ALL. Probably doesn't handle all illegal characters and does NOT uniquely identify a workspace.
	// https://issues.redhat.com/browse/CMCS-158 should ensure uniqueness and ensure a valid managed cluster set name is generated
	// TODO: incorporate kcp shard info
	return strings.ReplaceAll(strings.ReplaceAll(workspaceName, ":", "_"), "-", "_")
}

func GetSyncerPrefix() string {
	return "kcp-syncer"
}

func GetSyncerName(regClusterName string) string { // Should be passing in the SyncTarget
	//TODO - Adjust to match https://github.com/robinbobbitt/kcp/blob/b6314f86a563a354eddde44f1a7038042090df9e/pkg/cliplugins/workload/plugin/sync.go#L141 once we have SyncTarget
	return fmt.Sprintf("%s-%s", GetSyncerPrefix(), regClusterName)
}

func GetSyncerServiceAccountName() string {
	return "kcp-syncer-sa"
}
