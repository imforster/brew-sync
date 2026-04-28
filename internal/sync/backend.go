package sync

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const gitTimeout = 60 * time.Second

// SyncBackend abstracts pushing and pulling the manifest to/from a remote location.
type SyncBackend interface {
	// Pull fetches the manifest from the remote location and writes it to dest.
	Pull(dest string) error
	// Push reads the manifest from src and pushes it to the remote location.
	Push(src string) error
	// Name returns the name of the sync backend (e.g., "git", "file").
	Name() string
}

// GitBackend synchronizes the manifest via a Git repository.
type GitBackend struct {
	RepoURL string
	Branch  string
	WorkDir string        // temporary working directory for git operations
	Timeout time.Duration // per-operation timeout; defaults to gitTimeout if zero
}

func (g *GitBackend) timeout() time.Duration {
	if g.Timeout > 0 {
		return g.Timeout
	}
	return gitTimeout
}

// NewGitBackend creates a new GitBackend with the given repository URL, branch, and working directory.
func NewGitBackend(repoURL, branch, workDir string) *GitBackend {
	return &GitBackend{
		RepoURL: repoURL,
		Branch:  branch,
		WorkDir: workDir,
	}
}

// Name returns "git".
func (g *GitBackend) Name() string {
	return "git"
}

// Pull fetches the latest manifest from the Git repository.
// If the working directory doesn't exist or isn't a git repo, it clones the repository.
// If it already exists, it pulls the latest changes.
// The manifest file is then copied from the working directory to dest.
func (g *GitBackend) Pull(dest string) error {
	if isGitRepo(g.WorkDir) {
		if err := g.pull(); err != nil {
			// Repo may be corrupted; remove and re-clone.
			if removeErr := os.RemoveAll(g.WorkDir); removeErr != nil {
				return fmt.Errorf("git pull failed: %w (cleanup also failed: %v)", err, removeErr)
			}
			if cloneErr := g.clone(); cloneErr != nil {
				return fmt.Errorf("git pull failed and re-clone also failed: %w", cloneErr)
			}
		}
	} else {
		if err := g.clone(); err != nil {
			return fmt.Errorf("git clone failed: %w", err)
		}
	}

	// Copy the manifest file from the working directory to dest.
	srcFile := filepath.Join(g.WorkDir, filepath.Base(dest))
	return copyFile(srcFile, dest)
}

// Push copies the manifest from src into the working directory and commits/pushes it.
func (g *GitBackend) Push(src string) error {
	// Ensure the working directory has a cloned repo.
	if !isGitRepo(g.WorkDir) {
		if err := g.clone(); err != nil {
			return fmt.Errorf("git clone failed: %w", err)
		}
	} else {
		// Pull latest before pushing to reduce conflicts.
		if err := g.pull(); err != nil {
			return fmt.Errorf("git pull failed before push: %w", err)
		}
	}

	// Copy the manifest file into the working directory.
	destFile := filepath.Join(g.WorkDir, filepath.Base(src))
	if err := copyFile(src, destFile); err != nil {
		return fmt.Errorf("failed to copy manifest to git working directory: %w", err)
	}

	// Stage the manifest file.
	if err := g.gitCommand("add", filepath.Base(src)); err != nil {
		return fmt.Errorf("git add failed: %w", err)
	}

	// Skip commit and push when the manifest is unchanged since the last push.
	if !g.hasStagedChanges() {
		return nil
	}

	// Commit the changes.
	if err := g.gitCommand("commit", "-m", "Update brew-sync manifest"); err != nil {
		return fmt.Errorf("git commit failed: %w", err)
	}

	// Push to the remote.
	if err := g.gitCommand("push", "origin", g.Branch); err != nil {
		return fmt.Errorf("git push failed (check authentication and connectivity): %w", err)
	}

	return nil
}

// hasStagedChanges returns true if there are staged changes ready to be committed.
// It relies on `git diff --cached --quiet` exiting 1 when differences exist, 0 when there are none.
func (g *GitBackend) hasStagedChanges() bool {
	cmd := exec.Command("git", "diff", "--cached", "--quiet")
	cmd.Dir = g.WorkDir
	return cmd.Run() != nil
}

// clone clones the repository into the working directory.
func (g *GitBackend) clone() error {
	// Ensure the parent directory exists.
	if err := os.MkdirAll(filepath.Dir(g.WorkDir), 0o755); err != nil {
		return fmt.Errorf("failed to create parent directory for git working directory: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), g.timeout())
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "clone", "--branch", g.Branch, "--single-branch", g.RepoURL, g.WorkDir)
	cmd.WaitDelay = 3 * time.Second
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to clone %s (branch %s): %s: %w", g.RepoURL, g.Branch, string(output), err)
	}
	return nil
}

// pull runs git pull in the working directory.
func (g *GitBackend) pull() error {
	ctx, cancel := context.WithTimeout(context.Background(), g.timeout())
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "pull", "origin", g.Branch)
	cmd.Dir = g.WorkDir
	cmd.WaitDelay = 3 * time.Second
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to pull from %s (branch %s): %s: %w", g.RepoURL, g.Branch, string(output), err)
	}
	return nil
}

// gitCommand runs a git command in the working directory.
func (g *GitBackend) gitCommand(args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), g.timeout())
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = g.WorkDir
	cmd.WaitDelay = 3 * time.Second
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w", string(output), err)
	}
	return nil
}

// isGitRepo checks whether the given path is an existing git repository.
func isGitRepo(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	if err != nil {
		return false
	}
	return info.IsDir()
}

// copyFile copies a file from src to dest, creating parent directories as needed.
func copyFile(src, dest string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", src, err)
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("failed to create directory for %s: %w", dest, err)
	}

	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return fmt.Errorf("failed to write %s: %w", dest, err)
	}

	return nil
}
