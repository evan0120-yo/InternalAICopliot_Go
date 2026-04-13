package builder

import (
	"strings"

	"com.citrus.internalaicopilot/internal/aiclient"
	"com.citrus.internalaicopilot/internal/infra"
)

func chooseAIRouteCode(command ConsultCommand, builderConfig infra.BuilderConfig, defaultRoute aiclient.AIRouteCode) aiclient.AIRouteCode {
	if command.Mode == ConsultModeProfile {
		return aiclient.AIRouteDirectGPT54
	}

	switch strings.TrimSpace(builderConfig.BuilderCode) {
	case "line-memo-crud", "linebot-memo-crud", "line-event-extract", "line-event-parser":
		return aiclient.AIRouteDirectGemma
	default:
		switch defaultRoute {
		case aiclient.AIRouteDirectGemma, aiclient.AIRouteGemmaThenGPT54, aiclient.AIRouteDirectGPT54:
			return defaultRoute
		default:
			return aiclient.AIRouteDirectGPT54
		}
	}
}
