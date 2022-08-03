// Copyright Red Hat
package resources

import (
	"embed"

	"github.com/stolostron/applier/pkg/asset"
)

//go:embed cluster-registration compute-templates
var files embed.FS

func GetScenarioResourcesReader() *asset.ScenarioResourcesReader {
	return asset.NewScenarioResourcesReader(&files)
}
