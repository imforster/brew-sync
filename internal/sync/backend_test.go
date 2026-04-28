package sync

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"brew-sync/internal/config"
)

// --- File backend tests ---

func TestFileBackend_PushPull(t *testing.T) {
	// Set up a "remote" directory and a local source/dest.
	remoteDir := t.TempDir()
	remotePath := filepath.Join(remoteDir, "brew-sync.toml")

	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "brew-sync.toml")

	content := []byte("version = 1\ntaps = [\"homebrew/core\"]\n")
	if err := os.WriteFile(srcFile, content, 0o644); err != nil {
		t.Fatalf("failed to write source file: %v", err)
	}

	fb := NewFileBackend(remotePath)

	// Push the manifest to the remote location.
	if err := fb.Push(srcFile); err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	// Pull the manifest to a different destination.
	destDir := t.TempDir()
	destFile := filepath.Join(destDir, "pulled.toml")
	if err := fb.Pull(destFile); err != nil {
		t.Fatalf("Pull failed: %v", err)
	}

	got, err := os.ReadFile(destFile)
	if err != nil {
		t.Fatalf("failed to read pulled file: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("pulled content mismatch:\ngot:  %q\nwant: %q", string(got), string(content))
	}
}

func TestFileBackend_PullNotFound(t *testing.T) {
	fb := NewFileBackend("/nonexistent/path/brew-sync.toml")

	destDir := t.TempDir()
	destFile := filepath.Join(destDir, "pulled.toml")

	err := fb.Pull(destFile)
	if err == nil {
		t.Fatal("expected error when pulling from nonexistent path, got nil")
	}

	// Verify the error message is descriptive (mentions the path).
	errMsg := err.Error()
	if !contains(errMsg, "not found") {
		t.Errorf("expected descriptive error mentioning 'not found', got: %s", errMsg)
	}
}

func TestFileBackend_Name(t *testing.T) {
	fb := NewFileBackend("/some/path")
	if got := fb.Name(); got != "file" {
		t.Errorf("Name() = %q, want %q", got, "file")
	}
}

// --- Git backend tests ---

func TestGitBackend_PushPull(t *testing.T) {
	// Skip if git is not available.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH, skipping git backend test")
	}

	// Create a local bare repository.
	bareRepo := filepath.Join(t.TempDir(), "test-repo.git")
	if out, err := exec.Command("git", "init", "--bare", bareRepo).CombinedOutput(); err != nil {
		t.Fatalf("failed to create bare repo: %s: %v", string(out), err)
	}

	// Clone the bare repo into a working directory to set up an initial commit,
	// so the branch exists for the GitBackend to clone from.
	setupDir := filepath.Join(t.TempDir(), "setup")
	if out, err := exec.Command("git", "clone", bareRepo, setupDir).CombinedOutput(); err != nil {
		t.Fatalf("failed to clone bare repo for setup: %s: %v", string(out), err)
	}

	// Configure git user in the setup repo.
	for _, args := range [][]string{
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "Test"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = setupDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git config failed: %s: %v", string(out), err)
		}
	}

	// Create an initial commit so the main branch exists.
	readmePath := filepath.Join(setupDir, "README.md")
	if err := os.WriteFile(readmePath, []byte("init"), 0o644); err != nil {
		t.Fatalf("failed to write README: %v", err)
	}
	for _, args := range [][]string{
		{"add", "."},
		{"commit", "-m", "initial commit"},
		{"push", "origin", "main"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = setupDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %s: %v", args, string(out), err)
		}
	}

	// Now use the GitBackend to push a manifest.
	workDir := filepath.Join(t.TempDir(), "git-work")
	gb := NewGitBackend(bareRepo, "main", workDir)

	// Configure git user in the work dir after clone happens.
	// We need to push first, which will clone internally.
	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "brew-sync.toml")
	content := []byte("version = 1\ntaps = [\"homebrew/core\"]\n")
	if err := os.WriteFile(srcFile, content, 0o644); err != nil {
		t.Fatalf("failed to write source manifest: %v", err)
	}

	// The Push will clone the repo first. We need to configure git user
	// in the work dir after clone. So we clone manually first.
	if out, err := exec.Command("git", "clone", "--branch", "main", "--single-branch", bareRepo, workDir).CombinedOutput(); err != nil {
		t.Fatalf("failed to pre-clone for work dir: %s: %v", string(out), err)
	}
	for _, args := range [][]string{
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "Test"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = workDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git config in workdir failed: %s: %v", string(out), err)
		}
	}

	if err := gb.Push(srcFile); err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	// Pull the manifest to a different location.
	pullWorkDir := filepath.Join(t.TempDir(), "git-pull-work")
	gbPull := NewGitBackend(bareRepo, "main", pullWorkDir)

	destDir := t.TempDir()
	destFile := filepath.Join(destDir, "brew-sync.toml")
	if err := gbPull.Pull(destFile); err != nil {
		t.Fatalf("Pull failed: %v", err)
	}

	got, err := os.ReadFile(destFile)
	if err != nil {
		t.Fatalf("failed to read pulled file: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("pulled content mismatch:\ngot:  %q\nwant: %q", string(got), string(content))
	}
}

func TestGitBackend_DoublePushNoError(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}

	bareRepo := filepath.Join(t.TempDir(), "repo.git")
	if out, err := exec.Command("git", "init", "--bare", bareRepo).CombinedOutput(); err != nil {
		t.Fatalf("bare init: %s: %v", out, err)
	}

	// Seed the bare repo with an initial commit so the branch exists.
	setupDir := filepath.Join(t.TempDir(), "setup")
	if out, err := exec.Command("git", "clone", bareRepo, setupDir).CombinedOutput(); err != nil {
		t.Fatalf("clone setup: %s: %v", out, err)
	}
	for _, args := range [][]string{
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "Test"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = setupDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("config: %s: %v", out, err)
		}
	}
	if err := os.WriteFile(filepath.Join(setupDir, "README.md"), []byte("init"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "."}, {"commit", "-m", "init"}, {"push", "origin", "main"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = setupDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
	}

	// Pre-clone work dir and configure git user.
	workDir := filepath.Join(t.TempDir(), "work")
	if out, err := exec.Command("git", "clone", "--branch", "main", "--single-branch", bareRepo, workDir).CombinedOutput(); err != nil {
		t.Fatalf("clone work: %s: %v", out, err)
	}
	for _, args := range [][]string{
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "Test"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = workDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("config work: %s: %v", out, err)
		}
	}

	srcFile := filepath.Join(t.TempDir(), "brew-sync.toml")
	if err := os.WriteFile(srcFile, []byte("version = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	gb := NewGitBackend(bareRepo, "main", workDir)

	if err := gb.Push(srcFile); err != nil {
		t.Fatalf("first push: %v", err)
	}

	// Second push with identical content must succeed (issue #5).
	if err := gb.Push(srcFile); err != nil {
		t.Fatalf("second push should be a no-op but failed: %v", err)
	}
}

func TestGitBackend_PullReclonesOnCorruptedRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH, skipping git backend test")
	}

	// Create a bare repo with a manifest.
	bareRepo := filepath.Join(t.TempDir(), "test-repo.git")
	if out, err := exec.Command("git", "init", "--bare", bareRepo).CombinedOutput(); err != nil {
		t.Fatalf("failed to create bare repo: %s: %v", string(out), err)
	}

	setupDir := filepath.Join(t.TempDir(), "setup")
	if out, err := exec.Command("git", "clone", bareRepo, setupDir).CombinedOutput(); err != nil {
		t.Fatalf("failed to clone bare repo: %s: %v", string(out), err)
	}
	for _, args := range [][]string{
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "Test"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = setupDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git config failed: %s: %v", string(out), err)
		}
	}

	content := []byte("version = 1\ntaps = [\"homebrew/core\"]\n")
	if err := os.WriteFile(filepath.Join(setupDir, "brew-sync.toml"), content, 0o644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}
	for _, args := range [][]string{
		{"add", "."},
		{"commit", "-m", "add manifest"},
		{"push", "origin", "main"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = setupDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %s: %v", args, string(out), err)
		}
	}

	// Create a work dir with a corrupted .git directory.
	workDir := filepath.Join(t.TempDir(), "git-work")
	if err := os.MkdirAll(filepath.Join(workDir, ".git"), 0o755); err != nil {
		t.Fatalf("failed to create corrupted .git: %v", err)
	}

	gb := NewGitBackend(bareRepo, "main", workDir)
	destFile := filepath.Join(t.TempDir(), "brew-sync.toml")

	if err := gb.Pull(destFile); err != nil {
		t.Fatalf("Pull should recover from corrupted repo, got: %v", err)
	}

	got, err := os.ReadFile(destFile)
	if err != nil {
		t.Fatalf("failed to read pulled file: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("pulled content mismatch:\ngot:  %q\nwant: %q", string(got), string(content))
	}
}

func TestGitBackend_CloneTimesOut(t *testing.T) {
	// Create a fake "git" that sleeps forever.
	fakeGit := filepath.Join(t.TempDir(), "git")
	if err := os.WriteFile(fakeGit, []byte("#!/bin/sh\nsleep 60\n"), 0o755); err != nil {
		t.Fatalf("failed to write fake git: %v", err)
	}

	// Put fake git first in PATH.
	t.Setenv("PATH", filepath.Dir(fakeGit)+":"+os.Getenv("PATH"))

	workDir := filepath.Join(t.TempDir(), "git-work")
	gb := NewGitBackend("https://example.com/repo.git", "main", workDir)
	gb.Timeout = 1 * time.Second

	destFile := filepath.Join(t.TempDir(), "brew-sync.toml")

	start := time.Now()
	err := gb.Pull(destFile)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if elapsed > 5*time.Second {
		t.Errorf("expected ~1s timeout, but took %s", elapsed)
	}
}

func TestGitBackend_Name(t *testing.T) {
	gb := NewGitBackend("https://example.com/repo.git", "main", "/tmp/work")
	if got := gb.Name(); got != "git" {
		t.Errorf("Name() = %q, want %q", got, "git")
	}
}

// --- Factory tests ---

func TestNewSyncBackend_Git(t *testing.T) {
	cfg := &config.Config{
		SyncBackend: "git",
		Git: config.GitConfig{
			RepoURL: "https://example.com/repo.git",
			Branch:  "main",
		},
	}

	backend, err := NewSyncBackend(cfg)
	if err != nil {
		t.Fatalf("NewSyncBackend returned error: %v", err)
	}
	if backend.Name() != "git" {
		t.Errorf("expected git backend, got %q", backend.Name())
	}

	gb, ok := backend.(*GitBackend)
	if !ok {
		t.Fatalf("expected *GitBackend, got %T", backend)
	}
	if gb.RepoURL != cfg.Git.RepoURL {
		t.Errorf("RepoURL = %q, want %q", gb.RepoURL, cfg.Git.RepoURL)
	}
	if gb.Branch != cfg.Git.Branch {
		t.Errorf("Branch = %q, want %q", gb.Branch, cfg.Git.Branch)
	}
}

func TestNewSyncBackend_File(t *testing.T) {
	cfg := &config.Config{
		SyncBackend: "file",
		File: config.FileConfig{
			RemotePath: "/shared/brew-sync.toml",
		},
	}

	backend, err := NewSyncBackend(cfg)
	if err != nil {
		t.Fatalf("NewSyncBackend returned error: %v", err)
	}
	if backend.Name() != "file" {
		t.Errorf("expected file backend, got %q", backend.Name())
	}

	fb, ok := backend.(*FileBackend)
	if !ok {
		t.Fatalf("expected *FileBackend, got %T", backend)
	}
	if fb.RemotePath != cfg.File.RemotePath {
		t.Errorf("RemotePath = %q, want %q", fb.RemotePath, cfg.File.RemotePath)
	}
}

func TestNewSyncBackend_Unsupported(t *testing.T) {
	cfg := &config.Config{
		SyncBackend: "s3",
	}

	backend, err := NewSyncBackend(cfg)
	if err == nil {
		t.Fatal("expected error for unsupported backend, got nil")
	}
	if backend != nil {
		t.Errorf("expected nil backend, got %v", backend)
	}

	errMsg := err.Error()
	if !contains(errMsg, "unsupported") {
		t.Errorf("expected error to mention 'unsupported', got: %s", errMsg)
	}
	if !contains(errMsg, "s3") {
		t.Errorf("expected error to mention 's3', got: %s", errMsg)
	}
}

// contains checks if s contains substr (case-sensitive).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
