// Copyright Red Hat
package hack

import (
	"embed"

	"github.com/stolostron/applier/pkg/asset"
)

//go:embed compute
var files embed.FS

func GetScenarioResourcesReader() *asset.ScenarioResourcesReader {
	return asset.NewScenarioResourcesReader(&files)
}
