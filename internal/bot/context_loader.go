package bot

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

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
