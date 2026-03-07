package bot

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const botContextHashKey = "codex.bot_context_hash"

func (a *App) syncBotContext() error {
	capabilities := buildCapabilitiesContext(a.tools.Specs())
	toolGuide := buildToolGuideContext(a.tools.Specs())
	autonomyGuide := buildAutonomyGuideContext()
	workspaceGuide := buildWorkspaceGuideContext()
	philosophyGuide := buildPhilosophyGuideContext()
	selfModelGuide := buildSelfModelGuideContext()
	epistemicGuide := buildEpistemicGuideContext()
	relationGuide := buildRelationGuideContext()
	memoryGuide := buildMemoryGuideContext()
	loopsGuide := buildLoopsGuideContext()
	timingGuide := buildTimingGuideContext()
	failureGuide := buildFailureGuideContext()
	if err := os.MkdirAll(a.paths.WorkspaceContextDir, 0o755); err != nil {
		return fmt.Errorf("create bot context dir: %w", err)
	}
	files := []struct {
		path    string
		content string
		label   string
	}{
		{path: a.paths.WorkspaceCapabilitiesPath, content: capabilities, label: "capabilities"},
		{path: filepath.Join(a.paths.WorkspaceContextDir, "tools.md"), content: toolGuide, label: "tools"},
		{path: filepath.Join(a.paths.WorkspaceContextDir, "autonomy.md"), content: autonomyGuide, label: "autonomy"},
		{path: filepath.Join(a.paths.WorkspaceContextDir, "workspace.md"), content: workspaceGuide, label: "workspace"},
		{path: filepath.Join(a.paths.WorkspaceContextDir, "philosophy.md"), content: philosophyGuide, label: "philosophy"},
		{path: filepath.Join(a.paths.WorkspaceContextDir, "self_model.md"), content: selfModelGuide, label: "self_model"},
		{path: filepath.Join(a.paths.WorkspaceContextDir, "epistemics.md"), content: epistemicGuide, label: "epistemics"},
		{path: filepath.Join(a.paths.WorkspaceContextDir, "relation.md"), content: relationGuide, label: "relation"},
		{path: filepath.Join(a.paths.WorkspaceContextDir, "memory.md"), content: memoryGuide, label: "memory"},
		{path: filepath.Join(a.paths.WorkspaceContextDir, "loops.md"), content: loopsGuide, label: "loops"},
		{path: filepath.Join(a.paths.WorkspaceContextDir, "timing.md"), content: timingGuide, label: "timing"},
		{path: filepath.Join(a.paths.WorkspaceContextDir, "failure.md"), content: failureGuide, label: "failure"},
	}
	for _, file := range files {
		if err := os.WriteFile(file.path, []byte(file.content), 0o644); err != nil {
			return fmt.Errorf("write %s context: %w", file.label, err)
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
