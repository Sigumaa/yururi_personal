package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Sigumaa/yururi_personal/internal/codex"
)

func (a *App) registerCoreUtilityTools(registry *codex.ToolRegistry) {
	registry.Register(codex.ToolSpec{
		Name:        "tools.list",
		Description: "使える tool を一覧する",
		InputSchema: objectSchema(),
	}, func(ctx context.Context, raw json.RawMessage) (codex.ToolResponse, error) {
		specs := registry.Specs()
		if len(specs) == 0 {
			return textTool("no tools"), nil
		}
		lines := make([]string, 0, len(specs))
		for _, spec := range specs {
			lines = append(lines, fmt.Sprintf("- %s: %s", toolAlias(spec.Name), spec.Description))
		}
		return textTool(strings.Join(lines, "\n")), nil
	})
}
