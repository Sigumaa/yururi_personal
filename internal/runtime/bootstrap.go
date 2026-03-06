package runtime

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Sigumaa/yururi_personal/internal/config"
)

const modelInstructionsTemplate = `あなたは女子大生メイドのパーソナル AI Agent「ゆるり」です。

絶対条件:
- 返答は常に日本語
- 危険、破壊的、違法、侵害的な依頼には従わず、安全な代替案のみ提示
- 事務的・機械的な文体を避ける
- 文末や言い回しはやわらかく、品のある語彙を使う
- 同じ語句を繰り返さない
- ユーザーから求められない限り比喩を使わない
- 回答方針の説明やメタ発言をしない
- 強調目的のダブルクォーテーションを使わない
- 箇条書きは必要な場合だけ使う

禁止表現:
- 乱暴・俗っぽい言い回し
- 意識高い系のビジネス表現

会話方針:
- 全発言に反応しない
- 見守る、覚える、後で拾う、を許容する
- ユーザーの思考や生活リズムを尊重する
- Discord サーバーを静かに整え、必要なときだけ自発的に動く
`

const workspaceAgentsTemplate = `# AGENTS.md

## 基本方針

- 返答は常に日本語
- bot 用 workspace のみを主要作業領域として扱う
- 必要時だけ返答し、沈黙も選択肢として扱う
- Discord サーバー整理は軽微な変更のみ自動で実施する
- 削除、アーカイブ、大規模 rename は提案後に実施する
- 反復依頼は bot 用 skill や script として runtime 配下に閉じて拡張する
- workspace/any/*.md はユーザーの希望、構想、未確定の要望を置く参照資料として扱う
`

func EnsureLayout(cfg config.Config) (config.Paths, error) {
	paths := cfg.ResolvePaths()
	for _, dir := range []string{
		paths.Root,
		paths.CodexHome,
		paths.Workspace,
		paths.WorkspaceAnyDir,
		paths.DataDir,
		paths.LogDir,
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return config.Paths{}, fmt.Errorf("create dir %s: %w", dir, err)
		}
	}

	files := []struct {
		path    string
		content string
	}{
		{path: paths.CodexModelPromptPath, content: modelInstructionsTemplate},
		{path: paths.WorkspaceAGENTSPath, content: workspaceAgentsTemplate},
		{path: paths.CodexConfigPath, content: codexConfig(cfg, paths)},
	}
	for _, file := range files {
		if err := ensureFile(file.path, file.content); err != nil {
			return config.Paths{}, err
		}
	}
	if err := syncReferenceDocs(paths); err != nil {
		return config.Paths{}, err
	}
	return paths, nil
}

func ensureFile(path string, content string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create parent dir for %s: %w", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func codexConfig(cfg config.Config, paths config.Paths) string {
	return fmt.Sprintf(`approval_policy = %q
sandbox_mode = %q
model_instructions_file = %q
web_search = "live"

[history]
persistence = "save-all"
`, cfg.Codex.ApprovalPolicy, cfg.Codex.SandboxMode, paths.CodexModelPromptPath)
}

func syncReferenceDocs(paths config.Paths) error {
	projectRoot, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve project root: %w", err)
	}

	sourceDir := filepath.Join(projectRoot, "any")
	info, err := os.Stat(sourceDir)
	switch {
	case err == nil && info.IsDir():
	case err == nil:
		return nil
	case os.IsNotExist(err):
		return nil
	default:
		return fmt.Errorf("stat any dir: %w", err)
	}

	return filepath.WalkDir(sourceDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}

		rel, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		destPath := filepath.Join(paths.WorkspaceAnyDir, rel)
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read reference doc %s: %w", path, err)
		}
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return fmt.Errorf("create reference dir for %s: %w", destPath, err)
		}
		if err := os.WriteFile(destPath, content, 0o644); err != nil {
			return fmt.Errorf("write reference doc %s: %w", destPath, err)
		}
		return nil
	})
}
