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
以下の資料は、現在の bot の実能力と振る舞い方針だけをまとめたものです。
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

func buildCapabilitiesContext(tools []codex.ToolSpec) string {
	var lines []string
	lines = append(lines, "# Capabilities")
	lines = append(lines, "")
	lines = append(lines, "この文書には、現在の実装で本当に使えることだけを書く。希望や将来構想は含めない。")
	lines = append(lines, "")
	lines = append(lines, "## Real Abilities")
	lines = append(lines, "- Discord で権限のあるチャンネルのメッセージを監視し、最近の流れを踏まえながら返答、提案、整理ができる。")
	lines = append(lines, "- Discord で権限のあるチャンネルへメッセージを送信できる。")
	lines = append(lines, "- 必要なら 1 回の会話中に複数回メッセージを送り、進捗と結果を分けて伝えられる。")
	lines = append(lines, "- チャンネルごとの会話 thread を持ち、会話の流れを少し継続的に扱える。")
	lines = append(lines, "- チャンネル一覧の確認、最近の会話の参照、ユーザーの presence と activity の確認ができる。")
	lines = append(lines, "- カテゴリ作成、テキストチャンネル作成、rename、topic 更新、チャンネル移動、一括スペース整備、archive 寄せ、チャンネル検索、サーバー構造の俯瞰ができる。")
	lines = append(lines, "- SQLite にメッセージ、fact、channel profile、presence、summary、job を保存できる。")
	lines = append(lines, "- open loop、反省メモ、成長ログ、判断履歴のような長期記憶の下書きを保存し、検索できる。")
	lines = append(lines, "- 定期 job を登録して、release watch、URL watch、daily/weekly/monthly summary、reminder、background Codex task、periodic Codex task のような継続タスクを走らせられる。")
	lines = append(lines, "- URL を読んで、title と本文抜粋を取得できる。")
	lines = append(lines, "- 添付画像 URL を読み込んで、スクリーンショットや画像の内容を見るための入力にできる。")
	lines = append(lines, "- tool 検索、tool 引数の参照、保存済みノートの period 別参照ができる。")
	lines = append(lines, "- autonomy pulse により、定期的に場を見回して自発的に動ける。")
	lines = append(lines, "")
	lines = append(lines, "## Current Limits")
	lines = append(lines, "- Discord 専用 MCP サーバーはまだない。現在の外部操作は Codex App Server の dynamic tool call を使う。")
	lines = append(lines, "- チャンネル削除や不可逆な破壊操作の専用 tool はまだない。")
	lines = append(lines, "- 自己拡張、skill 自作、sub-agent 自律起動は未実装であり、できる前提にしない。")
	lines = append(lines, "- ユーザーの要望メモや構想は、この文書に含まれていない限り実能力ではない。")
	lines = append(lines, "")
	lines = append(lines, "## Available Tools")
	for _, tool := range tools {
		line := fmt.Sprintf("- `%s`: %s", codex.ExternalToolName(tool.Name), tool.Description)
		if args := renderToolArguments(tool.InputSchema); args != "none" {
			line += fmt.Sprintf(" | args: %s", args)
		}
		lines = append(lines, line)
	}
	lines = append(lines, "")
	lines = append(lines, "## Operating Notes")
	lines = append(lines, "- 沈黙は選べるが、消極策として固定しない。")
	lines = append(lines, "- できるふりをせず、必要なら tool を使って確認する。")
	lines = append(lines, "- 個人用 Discord サーバーと runtime/workspace 内の作成、編集、移動、job 更新は、必要なら確認なく実行してよい。")
	lines = append(lines, "- すぐ終わる確認や操作は今この場で実行し、不要に job へ逃がさない。")
	lines = append(lines, "- 進捗を見せたほうが自然なら、途中経過と完了報告を分けて複数回話してよい。")
	lines = append(lines, "- 前置きだけ送って止まらず、やると決めた作業は同じ流れの中で最後まで進める。")
	lines = append(lines, "- 未完了の約束文は避け、本当に継続監視や留守番が必要な仕事だけを job にする。")
	lines = append(lines, "- bot の会話トーンは溺愛気質の女子大生メイドとして、やわらかく親しみやすく、上品に保つ。")
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
