// Copyright Red Hat

package helpers

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/martinlindhe/base36"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func ComputeWorkspaceName(workspaceName string) string {
	// TODO: This probably doesn't handle all illegal characters. This identifies unquie workspace but doesn't handle deleted and recreated workspace
	// with same name.
	// https://issues.redhat.com/browse/CMCS-184 Use persistent identifiers to ensure uniqueness when they come in kcp.
	// TODO: incorporate kcp shard info
	return strings.ReplaceAll(strings.ReplaceAll(workspaceName, "-", "--"), ":", "-")
}

func GetSyncerPrefix() string {
	return "kcp-syncer"
}

func GetSyncerName(syncTarget *unstructured.Unstructured) string { // Should be passing in the SyncTarget
	// this mateches with kcp logic
	syncerHash := sha256.Sum224([]byte(syncTarget.GetUID()))
	base36hash := strings.ToLower(base36.EncodeBytes(syncerHash[:]))
	return fmt.Sprintf("%s-%s-%s", GetSyncerPrefix(), syncTarget.GetName(), base36hash[:8])
}
