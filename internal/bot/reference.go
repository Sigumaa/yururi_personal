package bot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const referenceDocsHashKey = "codex.reference_docs_hash"

func (a *App) primeReferenceDocs(ctx context.Context) error {
	if a.thread.ID == "" {
		return nil
	}

	bundle, hash, err := loadReferenceDocs(a.paths.WorkspaceAnyDir)
	if err != nil {
		return err
	}
	if strings.TrimSpace(bundle) == "" {
		return nil
	}

	lastHash, ok, err := a.store.GetKV(ctx, referenceDocsHashKey)
	if err != nil {
		return err
	}
	if ok && lastHash == hash {
		return nil
	}

	prompt := fmt.Sprintf(`これは Discord に表示しない内部コンテキスト更新です。
workspace/any の reference docs が更新されました。

扱い方:
- これは実装済み機能の一覧ではない
- ユーザーの希望、理想像、未確定の構想、できたら嬉しいことが含まれる
- できる手段があるときに活かし、できないことは勝手にできるふりをしない
- 未実装のことを既に可能だと誤認しない
- 過剰な自律や過剰な整理を避けつつ、必要なときだけ自然に使う
- 内容を今後の会話判断と tool 利用の下地として取り込む

reference docs:
%s

返答は OK だけにしてください。`, bundle)

	a.codexMu.Lock()
	defer a.codexMu.Unlock()
	if _, err := a.codex.RunTurn(ctx, a.thread.ID, prompt); err != nil {
		return fmt.Errorf("prime reference docs turn: %w", err)
	}
	if err := a.store.SetKV(ctx, referenceDocsHashKey, hash); err != nil {
		return err
	}
	return nil
}

func loadReferenceDocs(dir string) (string, string, error) {
	entries := make([]string, 0)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return "", "", nil
	} else if err != nil {
		return "", "", fmt.Errorf("stat reference docs dir: %w", err)
	}

	type document struct {
		relPath string
		content string
	}
	var docs []document
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}

		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		docs = append(docs, document{
			relPath: filepath.ToSlash(rel),
			content: strings.TrimSpace(string(raw)),
		})
		return nil
	})
	if err != nil {
		return "", "", fmt.Errorf("walk reference docs: %w", err)
	}
	if len(docs) == 0 {
		return "", "", nil
	}

	slices.SortFunc(docs, func(a document, b document) int {
		switch {
		case a.relPath < b.relPath:
			return -1
		case a.relPath > b.relPath:
			return 1
		default:
			return 0
		}
	})

	for _, doc := range docs {
		entries = append(entries, fmt.Sprintf("## %s\n%s", doc.relPath, doc.content))
	}
	bundle := strings.Join(entries, "\n\n")
	sum := sha256.Sum256([]byte(bundle))
	return bundle, hex.EncodeToString(sum[:]), nil
}
