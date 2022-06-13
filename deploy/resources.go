// Copyright Red Hat
package deploy

import (
	"embed"

	"open-cluster-management.io/clusteradm/pkg/helpers/asset"
)

//go:embed compute-operator webhook
var files embed.FS

func GetScenarioResourcesReader() *asset.ScenarioResourcesReader {
	return asset.NewScenarioResourcesReader(&files)
}
