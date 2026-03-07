package bot

import "github.com/Sigumaa/yururi_personal/internal/codex"

func (a *App) registerMemoryAutonomyTools(registry *codex.ToolRegistry) {
	a.registerMemorySpecializedTools(registry)
	a.registerMemoryFactLifecycleTools(registry)
}
