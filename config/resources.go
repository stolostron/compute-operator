// Copyright Red Hat
package config

import (
	"embed"

	"github.com/stolostron/applier/pkg/asset"
)

//go:embed crd apiresourceschema
var files embed.FS

func GetScenarioResourcesReader() *asset.ScenarioResourcesReader {
	return asset.NewScenarioResourcesReader(&files)
}
