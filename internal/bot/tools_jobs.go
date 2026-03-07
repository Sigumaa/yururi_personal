package bot

import "github.com/Sigumaa/yururi_personal/internal/codex"

func (a *App) registerJobExtraTools(registry *codex.ToolRegistry) {
	a.registerJobStateTools(registry)
	a.registerJobScheduleTools(registry)
}
