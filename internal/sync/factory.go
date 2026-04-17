package sync

import (
	"fmt"
	"os"
	"path/filepath"

	"brew-sync/internal/config"
)

// NewSyncBackend returns the appropriate SyncBackend based on the provided configuration.
// It supports "git" and "file" backends. Returns an error for unsupported backend types.
func NewSyncBackend(cfg *config.Config) (SyncBackend, error) {
	switch cfg.SyncBackend {
	case "git":
		workDir := filepath.Join(os.TempDir(), "brew-sync-git")
		return NewGitBackend(cfg.Git.RepoURL, cfg.Git.Branch, workDir), nil
	case "file":
		return NewFileBackend(cfg.File.RemotePath), nil
	default:
		return nil, fmt.Errorf("unsupported sync backend: %s", cfg.SyncBackend)
	}
}
