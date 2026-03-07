package bot

import "github.com/Sigumaa/yururi_personal/internal/codex"

func (a *App) registerTools(registry *codex.ToolRegistry) {
	a.registerCoreUtilityTools(registry)
	a.registerCoreMemoryTools(registry)
	a.registerCoreJobTools(registry)
	a.registerCoreDiscordTools(registry)
	a.registerExtraTools(registry)
}
