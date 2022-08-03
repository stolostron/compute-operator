// Copyright Red Hat
package test

import (
	"embed"

	"github.com/stolostron/applier/pkg/asset"
)

//go:embed config resources
var files embed.FS

func GetScenarioResourcesReader() *asset.ScenarioResourcesReader {
	return asset.NewScenarioResourcesReader(&files)
}
