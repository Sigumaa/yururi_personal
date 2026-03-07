package bot

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Sigumaa/yururi_personal/internal/selfmodel"
)

const botContextHashKey = "codex.bot_context_hash"

func (a *App) syncBotContext() error {
	if err := os.MkdirAll(a.paths.WorkspaceContextDir, 0o755); err != nil {
		return fmt.Errorf("create bot context dir: %w", err)
	}

	for _, doc := range selfmodel.ManagedDocuments(a.tools.Specs()) {
		path := filepath.Join(a.paths.WorkspaceContextDir, doc.FileName)
		if doc.FileName == "capabilities.md" {
			path = a.paths.WorkspaceCapabilitiesPath
		}
		if err := os.WriteFile(path, []byte(doc.Content), 0o644); err != nil {
			return fmt.Errorf("write %s context: %w", doc.Label, err)
		}
	}
	return nil
}

func (a *App) primeBotContext(ctx context.Context) error {
	if a.thread.ID == "" {
		return nil
	}

	bundle, hash, err := loadBotContext(a.paths.WorkspaceContextDir)
	if err != nil {
		return err
	}
	if strings.TrimSpace(bundle) == "" {
		return nil
	}

	currentHash, ok, err := a.store.GetKV(ctx, botContextHashKey)
	if err != nil {
		return err
	}
	if ok && currentHash == hash {
		return nil
	}

	if err := a.primeThreadContext(ctx, a.thread.ID, bundle); err != nil {
		return fmt.Errorf("prime bot context: %w", err)
	}
	if err := a.store.SetKV(ctx, botContextHashKey, hash); err != nil {
		return fmt.Errorf("persist bot context hash: %w", err)
	}
	return nil
}

func (a *App) primeThreadContext(ctx context.Context, threadID string, bundle string) error {
	if threadID == "" || strings.TrimSpace(bundle) == "" {
		return nil
	}

	a.logger.Info("prime thread context start", "thread_id", threadID, "bundle_bytes", len(bundle))
	a.logger.Debug("prime thread context bundle", "thread_id", threadID, "bundle_preview", previewText(bundle, 1800))
	prompt := fmt.Sprintf(`これはユーザーへ見せない内部向けの context refresh です。
以下の資料は、現在の bot の実能力、道具の使いどころ、行動境界、workspace の使い方、自己認識、認識姿勢、関係の持ち方、記憶の意味、時間の扱い、失敗時の立て直しをまとめたものです。
古い思い込みや未実装の能力より、この資料を優先してください。
ここに書かれていない能力は、できる前提にしないでください。
この更新自体についてユーザーへ説明したり、会話を始めたりしないでください。
返答は OK のみ。

%s`, bundle)

	_, err := a.runThreadTurn(ctx, threadID, prompt)
	if err != nil {
		a.logger.Warn("prime thread context failed", "thread_id", threadID, "error", err)
		return err
	}
	a.logger.Info("prime thread context completed", "thread_id", threadID)
	return err
}
