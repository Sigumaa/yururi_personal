package bot

import "github.com/Sigumaa/yururi_personal/internal/codex"

func (a *App) registerDiscordReadWriteTools(registry *codex.ToolRegistry) {
	a.registerDiscordQueryTools(registry)
	a.registerDiscordSpaceManagementTools(registry)
}
