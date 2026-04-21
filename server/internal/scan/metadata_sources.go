package scan

import (
	"fmt"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

func MetadataSourceFromIntegration(integration *core.Integration) (MetadataSource, error) {
	if integration == nil {
		return MetadataSource{}, fmt.Errorf("integration is required")
	}
	config, err := parseConfig(integration.ConfigJSON)
	if err != nil {
		return MetadataSource{}, err
	}
	return MetadataSource{
		IntegrationID: integration.ID,
		Label:         integration.Label,
		PluginID:      integration.PluginID,
		Config:        config,
	}, nil
}
