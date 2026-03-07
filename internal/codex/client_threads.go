package codex

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/Sigumaa/yururi_personal/internal/config"
)

func (c *Client) EnsureThread(ctx context.Context, storedID string, baseInstructions string, developerInstructions string) (ThreadSession, error) {
	if storedID != "" {
		var response struct {
			Thread struct {
				ID string `json:"id"`
			} `json:"thread"`
		}
		err := c.call(ctx, "thread/resume", map[string]any{
			"threadId": storedID,
		}, &response)
		if err == nil && response.Thread.ID != "" {
			return ThreadSession{ID: response.Thread.ID}, nil
		}
		c.logger.Warn("thread/resume failed; starting fresh thread", "thread_id", storedID, "error", err)
	}

	var response struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	params := c.threadStartParams(baseInstructions, developerInstructions)
	if err := c.call(ctx, "thread/start", params, &response); err != nil {
		return ThreadSession{}, err
	}
	return ThreadSession{ID: response.Thread.ID}, nil
}

func (c *Client) DynamicToolSignature() string {
	if c == nil || c.tools == nil {
		return ""
	}
	dynamicTools := c.dynamicToolParams()
	if len(dynamicTools) == 0 {
		return ""
	}
	raw, err := json.Marshal(dynamicTools)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%x", sum)
}

func (c *Client) threadStartParams(baseInstructions string, developerInstructions string) map[string]any {
	params := map[string]any{
		"cwd":                   c.paths.Workspace,
		"approvalPolicy":        config.DefaultCodexApprovalPolicy,
		"sandbox":               config.DefaultCodexSandboxMode,
		"baseInstructions":      baseInstructions,
		"developerInstructions": developerInstructions,
		"serviceName":           c.cfg.AppName,
	}
	if dynamicTools := c.dynamicToolParams(); len(dynamicTools) > 0 {
		params["dynamicTools"] = dynamicTools
	}
	return params
}

func (c *Client) dynamicToolParams() []map[string]any {
	specs := c.tools.Specs()
	if len(specs) == 0 {
		return nil
	}

	out := make([]map[string]any, 0, len(specs))
	for _, spec := range specs {
		out = append(out, map[string]any{
			"name":        ExternalToolName(spec.Name),
			"description": spec.Description,
			"inputSchema": spec.InputSchema,
		})
	}
	return out
}
