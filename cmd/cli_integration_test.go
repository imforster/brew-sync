//go:build integration

package cmd_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// binaryPath is set by TestMain after building the brew-sync binary.
var binaryPath string

func TestMain(m *testing.M) {
	// Build the binary once for all tests.
	tmp, err := os.MkdirTemp("", "brew-sync-test-bin-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmp)

	binaryPath = filepath.Join(tmp, "brew-sync")
	build := exec.Command("go", "build", "-o", binaryPath, "..")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		panic("failed to build brew-sync binary: " + err.Error())
	}

	os.Exit(m.Run())
}

// run executes the brew-sync binary with the given args and returns combined output.
// It uses the provided config file via --config.
func run(t *testing.T, configPath string, args ...string) (string, error) {
	t.Helper()
	fullArgs := append([]string{"--config", configPath}, args...)
	cmd := exec.Command(binaryPath, fullArgs...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// writeConfig creates a config.toml in dir and returns its path.
func writeConfig(t *testing.T, dir, manifestPath, remotePath, machineTag string) string {
	t.Helper()
	content := "manifest_path = " + quote(manifestPath) + "\n"
	if machineTag != "" {
		content += "machine_tag = " + quote(machineTag) + "\n"
	}
	content += "sync_backend = \"file\"\n\n[file]\nremote_path = " + quote(remotePath) + "\n"
	p := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func quote(s string) string { return "\"" + s + "\"" }

// ---------------------------------------------------------------------------
// TestCLI_InitStatusApplyPushPull exercises the full happy-path workflow:
//   init → status → apply --dry-run → push → pull → status (on pulled copy)
// All commands are read-only or use the file backend; no Homebrew mutations.
// ---------------------------------------------------------------------------

func TestCLI_InitStatusApplyPushPull(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "brew-sync.toml")
	remote := filepath.Join(dir, "remote.toml")
	cfg := writeConfig(t, dir, manifest, remote, "test-machine")

	// 1. Init — captures real local Homebrew state
	out, err := run(t, cfg, "init")
	if err != nil {
		t.Fatalf("init failed: %s\n%s", err, out)
	}
	if !strings.Contains(out, "Manifest written to") {
		t.Fatalf("init: unexpected output: %s", out)
	}
	if _, err := os.Stat(manifest); err != nil {
		t.Fatalf("manifest not created: %v", err)
	}

	// 2. Status — should be mostly in sync (0 to install, 0 to upgrade)
	out, err = run(t, cfg, "status")
	if err != nil {
		t.Fatalf("status failed: %s\n%s", err, out)
	}
	if !strings.Contains(out, "0 packages to install") {
		t.Errorf("status: expected 0 to install, got: %s", out)
	}
	if !strings.Contains(out, "0 packages to upgrade") {
		t.Errorf("status: expected 0 to upgrade, got: %s", out)
	}

	// 3. Apply --dry-run — no mutations
	out, err = run(t, cfg, "apply", "--dry-run")
	if err != nil {
		t.Fatalf("apply --dry-run failed: %s\n%s", err, out)
	}
	if !strings.Contains(out, "Would install 0") {
		t.Errorf("apply --dry-run: unexpected output: %s", out)
	}

	// 4. Push — copies manifest to "remote" via file backend
	out, err = run(t, cfg, "push")
	if err != nil {
		t.Fatalf("push failed: %s\n%s", err, out)
	}
	if !strings.Contains(out, "pushed successfully") {
		t.Errorf("push: unexpected output: %s", out)
	}
	if _, err := os.Stat(remote); err != nil {
		t.Fatalf("remote manifest not created after push: %v", err)
	}

	// 5. Pull — fetch into a different location
	pullDir := t.TempDir()
	pullManifest := filepath.Join(pullDir, "brew-sync.toml")
	pullCfg := writeConfig(t, pullDir, pullManifest, remote, "other-machine")

	out, err = run(t, pullCfg, "pull")
	if err != nil {
		t.Fatalf("pull failed: %s\n%s", err, out)
	}
	if !strings.Contains(out, "pulled successfully") {
		t.Errorf("pull: unexpected output: %s", out)
	}

	// 6. Verify pulled manifest matches original
	origData, _ := os.ReadFile(remote)
	pullData, _ := os.ReadFile(pullManifest)
	if string(origData) != string(pullData) {
		t.Error("pulled manifest does not match pushed manifest")
	}

	// 7. Status on pulled manifest — same machine, should be in sync
	out, err = run(t, pullCfg, "status")
	if err != nil {
		t.Fatalf("status after pull failed: %s\n%s", err, out)
	}
	if !strings.Contains(out, "0 packages to install") {
		t.Errorf("status after pull: expected 0 to install, got: %s", out)
	}
}

// ---------------------------------------------------------------------------
// TestCLI_MergePreservesRemotePackages verifies that merge adds local-only
// packages without dropping packages from other machines.
// ---------------------------------------------------------------------------

func TestCLI_MergePreservesRemotePackages(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "brew-sync.toml")
	remote := filepath.Join(dir, "remote.toml")
	cfg := writeConfig(t, dir, manifestPath, remote, "test-machine")

	// Init to get a baseline manifest
	out, err := run(t, cfg, "init")
	if err != nil {
		t.Fatalf("init failed: %s\n%s", err, out)
	}

	// Read the manifest, inject a fake "other-machine-only" package, write it back
	data, _ := os.ReadFile(manifestPath)
	injected := string(data) + "\n[[formulae]]\n  name = \"fake-other-machine-pkg\"\n  only_on = [\"other-machine\"]\n"
	os.WriteFile(manifestPath, []byte(injected), 0o644)

	// Merge — should preserve fake-other-machine-pkg
	out, err = run(t, cfg, "merge")
	if err != nil {
		t.Fatalf("merge failed: %s\n%s", err, out)
	}
	if !strings.Contains(out, "Manifest merged") {
		t.Errorf("merge: unexpected output: %s", out)
	}

	// Verify the injected package survived the merge
	merged, _ := os.ReadFile(manifestPath)
	if !strings.Contains(string(merged), "fake-other-machine-pkg") {
		t.Error("merge dropped fake-other-machine-pkg — should preserve remote-only packages")
	}
}

// ---------------------------------------------------------------------------
// TestCLI_MachineTagFiltering verifies that status respects only_on/except_on
// when a machine_tag is configured.
// ---------------------------------------------------------------------------

func TestCLI_MachineTagFiltering(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "brew-sync.toml")
	remote := filepath.Join(dir, "remote.toml")
	cfg := writeConfig(t, dir, manifestPath, remote, "work-laptop")

	// Init to get a real manifest
	out, err := run(t, cfg, "init")
	if err != nil {
		t.Fatalf("init failed: %s\n%s", err, out)
	}

	// Inject packages with machine filters
	data, _ := os.ReadFile(manifestPath)
	extra := "\n[[formulae]]\n  name = \"only-on-home\"\n  only_on = [\"home-desktop\"]\n"
	extra += "\n[[formulae]]\n  name = \"except-work\"\n  except_on = [\"work-laptop\"]\n"
	os.WriteFile(manifestPath, append(data, []byte(extra)...), 0o644)

	// Status with machine_tag=work-laptop — filtered packages should NOT appear as "to install"
	out, err = run(t, cfg, "status", "--verbose")
	if err != nil {
		t.Fatalf("status failed: %s\n%s", err, out)
	}
	if strings.Contains(out, "only-on-home") {
		t.Error("only-on-home should be filtered out for work-laptop")
	}
	if strings.Contains(out, "except-work") {
		t.Error("except-work should be filtered out for work-laptop")
	}
}

// ---------------------------------------------------------------------------
// TestCLI_ErrorMissingManifest verifies that status/apply fail gracefully
// when no manifest exists.
// ---------------------------------------------------------------------------

func TestCLI_ErrorMissingManifest(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "nonexistent", "brew-sync.toml")
	remote := filepath.Join(dir, "remote.toml")
	cfg := writeConfig(t, dir, manifestPath, remote, "")

	out, err := run(t, cfg, "status")
	if err == nil {
		t.Fatal("expected error for missing manifest, got nil")
	}
	if !strings.Contains(out, "manifest not found") {
		t.Errorf("expected 'manifest not found' error, got: %s", out)
	}

	out, err = run(t, cfg, "apply")
	if err == nil {
		t.Fatal("expected error for missing manifest on apply, got nil")
	}
	if !strings.Contains(out, "manifest not found") {
		t.Errorf("expected 'manifest not found' error, got: %s", out)
	}
}

// ---------------------------------------------------------------------------
// TestCLI_ErrorUnreachableBackend verifies that pull fails gracefully when
// the file backend path doesn't exist.
// ---------------------------------------------------------------------------

func TestCLI_ErrorUnreachableBackend(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "brew-sync.toml")
	remote := "/nonexistent/path/remote.toml"
	cfg := writeConfig(t, dir, manifestPath, remote, "")

	out, err := run(t, cfg, "pull")
	if err == nil {
		t.Fatal("expected error for unreachable backend, got nil")
	}
	if !strings.Contains(out, "not found") {
		t.Errorf("expected 'not found' in error, got: %s", out)
	}
}
