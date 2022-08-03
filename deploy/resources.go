// Copyright Red Hat
package deploy

import (
	"embed"

	"github.com/stolostron/applier/pkg/asset"
)

//go:embed compute-operator webhook
var files embed.FS

func GetScenarioResourcesReader() *asset.ScenarioResourcesReader {
	return asset.NewScenarioResourcesReader(&files)
}
