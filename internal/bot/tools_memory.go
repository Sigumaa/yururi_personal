package bot

import "github.com/Sigumaa/yururi_personal/internal/codex"

func (a *App) registerMemoryExtraTools(registry *codex.ToolRegistry) {
	a.registerMemoryFactTools(registry)
	a.registerMemorySummaryTools(registry)
	a.registerMemoryProfileTools(registry)
	a.registerMemoryMessageTools(registry)
	a.registerMemoryVoiceTools(registry)
}
