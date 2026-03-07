package bot

import "github.com/Sigumaa/yururi_personal/internal/codex"

func (a *App) registerDiscordExtraTools(registry *codex.ToolRegistry) {
	a.registerDiscordReadWriteTools(registry)
	a.registerDiscordAnalysisTools(registry)
}
