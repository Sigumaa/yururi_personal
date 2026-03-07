package bot

import "github.com/Sigumaa/yururi_personal/internal/codex"

func (a *App) registerDiscordAutonomyTools(registry *codex.ToolRegistry) {
	a.registerDiscordAutonomyReadTools(registry)
	a.registerDiscordAutonomySnapshotTools(registry)
}
