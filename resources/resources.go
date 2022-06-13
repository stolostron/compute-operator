// Copyright Red Hat
package resources

import (
	"embed"

	"open-cluster-management.io/clusteradm/pkg/helpers/asset"
)

//go:embed compute-operator workspace
var files embed.FS

func GetScenarioResourcesReader() *asset.ScenarioResourcesReader {
	return asset.NewScenarioResourcesReader(&files)
}
