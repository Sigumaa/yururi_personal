package bot

import "github.com/Sigumaa/yururi_personal/internal/codex"

func (a *App) registerAutonomyTools(registry *codex.ToolRegistry) {
	a.registerMemoryAutonomyTools(registry)
	a.registerJobAutonomyTools(registry)
	a.registerDiscordAutonomyTools(registry)
}
