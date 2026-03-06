package runtime

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Sigumaa/yururi_personal/internal/config"
)

type ResetReport struct {
	Root    string
	Full    bool
	Removed []string
}

func Reset(cfg config.Config, full bool) (ResetReport, error) {
	paths := cfg.ResolvePaths()
	report := ResetReport{
		Root: paths.Root,
		Full: full,
	}

	if full {
		if err := removePath(paths.Root); err != nil {
			return report, err
		}
		report.Removed = append(report.Removed, paths.Root)
		_, err := EnsureLayout(cfg)
		return report, err
	}

	targets := []string{
		paths.DataDir,
		paths.LogDir,
		paths.Workspace,
		filepath.Join(paths.CodexHome, "shell_snapshots"),
	}
	for _, target := range targets {
		if err := removePath(target); err != nil {
			return report, err
		}
		report.Removed = append(report.Removed, target)
	}

	matches, err := filepath.Glob(filepath.Join(paths.CodexHome, "state_*.sqlite*"))
	if err != nil {
		return report, fmt.Errorf("glob codex state files: %w", err)
	}
	for _, match := range matches {
		if err := removePath(match); err != nil {
			return report, err
		}
		report.Removed = append(report.Removed, match)
	}

	_, err = EnsureLayout(cfg)
	return report, err
}

func removePath(path string) error {
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}
