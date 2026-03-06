package bot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Sigumaa/yururi_personal/internal/codex"
)

const botContextHashKey = "codex.bot_context_hash"

func (a *App) syncBotContext() error {
	capabilities := buildCapabilitiesContext(a.tools.Specs())
	if err := os.MkdirAll(a.paths.WorkspaceContextDir, 0o755); err != nil {
		return fmt.Errorf("create bot context dir: %w", err)
	}
	if err := os.WriteFile(a.paths.WorkspaceCapabilitiesPath, []byte(capabilities), 0o644); err != nil {
		return fmt.Errorf("write capabilities context: %w", err)
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

	prompt := fmt.Sprintf(`これはユーザーへ見せない内部向けの context refresh です。
以下の資料は、現在の bot の実能力と振る舞い方針だけをまとめたものです。
古い思い込みや未実装の能力より、この資料を優先してください。
ここに書かれていない能力は、できる前提にしないでください。
この更新自体についてユーザーへ説明したり、会話を始めたりしないでください。
返答は OK のみ。

%s`, bundle)

	a.codexMu.Lock()
	defer a.codexMu.Unlock()

	if _, err := a.codex.RunTurn(ctx, a.thread.ID, prompt); err != nil {
		return fmt.Errorf("prime bot context: %w", err)
	}
	if err := a.store.SetKV(ctx, botContextHashKey, hash); err != nil {
		return fmt.Errorf("persist bot context hash: %w", err)
	}
	return nil
}

func buildCapabilitiesContext(tools []codex.ToolSpec) string {
	var lines []string
	lines = append(lines, "# Capabilities")
	lines = append(lines, "")
	lines = append(lines, "この文書には、現在の実装で本当に使えることだけを書く。希望や将来構想は含めない。")
	lines = append(lines, "")
	lines = append(lines, "## Real Abilities")
	lines = append(lines, "- Discord で権限のあるチャンネルのメッセージを監視し、必要なときだけ返答できる。")
	lines = append(lines, "- Discord で権限のあるチャンネルへメッセージを送信できる。")
	lines = append(lines, "- チャンネル一覧の確認、最近の会話の参照、ユーザーの presence と activity の確認ができる。")
	lines = append(lines, "- カテゴリ作成、テキストチャンネル作成、チャンネル移動ができる。")
	lines = append(lines, "- SQLite にメッセージ、fact、channel profile、presence、summary、job を保存できる。")
	lines = append(lines, "- 定期 job を登録して、release watch や summary のような継続タスクを走らせられる。")
	lines = append(lines, "")
	lines = append(lines, "## Current Limits")
	lines = append(lines, "- Discord 専用 MCP サーバーはまだない。現在の外部操作は Codex App Server の dynamic tool call を使う。")
	lines = append(lines, "- チャンネル削除、アーカイブ、rename の専用 tool はまだない。大きな整理は提案を優先する。")
	lines = append(lines, "- 自己拡張、skill 自作、sub-agent 自律起動は未実装であり、できる前提にしない。")
	lines = append(lines, "- ユーザーの要望メモや構想は、この文書に含まれていない限り実能力ではない。")
	lines = append(lines, "")
	lines = append(lines, "## Available Tools")
	for _, tool := range tools {
		line := fmt.Sprintf("- `%s`: %s", tool.Name, tool.Description)
		if args := renderToolArguments(tool.InputSchema); args != "none" {
			line += fmt.Sprintf(" | args: %s", args)
		}
		lines = append(lines, line)
	}
	lines = append(lines, "")
	lines = append(lines, "## Operating Notes")
	lines = append(lines, "- 沈黙は失敗ではない。必要なときだけ返答する。")
	lines = append(lines, "- できるふりをせず、必要なら tool を使って確認する。")
	lines = append(lines, "- bot の会話トーンは女子大生メイドとして、やわらかく親しみやすく、上品に保つ。")
	return strings.Join(lines, "\n")
}

func loadBotContext(dir string) (string, string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return "", "", fmt.Errorf("walk bot context: %w", err)
	}
	sort.Strings(files)

	var sections []string
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", "", fmt.Errorf("read bot context %s: %w", path, err)
		}
		body := strings.TrimSpace(string(raw))
		if body == "" {
			continue
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return "", "", fmt.Errorf("rel bot context %s: %w", path, err)
		}
		sections = append(sections, fmt.Sprintf("## %s\n\n%s", filepath.ToSlash(rel), body))
	}

	bundle := strings.Join(sections, "\n\n")
	sum := sha256.Sum256([]byte(bundle))
	return bundle, hex.EncodeToString(sum[:]), nil
}
