package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"brew-sync/internal/brew"
	"brew-sync/internal/diff"
	"brew-sync/internal/manifest"
	"brew-sync/internal/sync"
)

// ---------------------------------------------------------------------------
// Integration Test: Full workflow init → push → pull → status → apply
// Validates: Requirements 1.1, 2.1, 4.1, 6.1, 7.1
// ---------------------------------------------------------------------------

func TestIntegration_FullWorkflow(t *testing.T) {
	// Set up MockBrewRunner with known state
	mock := brew.NewMockBrewRunner()
	mock.Formulae = []diff.Package{
		{Name: "git", Version: "2.40"},
		{Name: "go", Version: "1.23"},
		{Name: "curl", Version: "8.0"},
	}
	mock.Casks = []diff.Package{
		{Name: "firefox", Version: "120.0"},
	}
	mock.Taps = []string{"homebrew/core", "homebrew/cask"}

	manager := manifest.NewManifestManager()

	// --- Phase 1: Init ---
	// Build manifest from mock local state (simulates `brew-sync init`)
	formulae, err := mock.ListFormulae()
	if err != nil {
		t.Fatalf("ListFormulae failed: %v", err)
	}
	casks, err := mock.ListCasks()
	if err != nil {
		t.Fatalf("ListCasks failed: %v", err)
	}
	taps, err := mock.ListTaps()
	if err != nil {
		t.Fatalf("ListTaps failed: %v", err)
	}

	localFormulae := make([]manifest.LocalPackage, len(formulae))
	for i, pkg := range formulae {
		localFormulae[i] = manifest.LocalPackage{Name: pkg.Name, Version: pkg.Version}
	}
	localCasks := make([]manifest.LocalPackage, len(casks))
	for i, pkg := range casks {
		localCasks[i] = manifest.LocalPackage{Name: pkg.Name, Version: pkg.Version}
	}

	m := manager.BuildFromLocal(localFormulae, localCasks, taps)

	// Save manifest to a temp "local" directory
	localDir := t.TempDir()
	localManifestPath := filepath.Join(localDir, "brew-sync.toml")
	if err := manager.Save(localManifestPath, m); err != nil {
		t.Fatalf("Save manifest failed: %v", err)
	}

	// Verify manifest was created with correct counts
	if len(m.Formulae) != 3 {
		t.Errorf("Init: expected 3 formulae, got %d", len(m.Formulae))
	}
	if len(m.Casks) != 1 {
		t.Errorf("Init: expected 1 cask, got %d", len(m.Casks))
	}
	if len(m.Taps) != 2 {
		t.Errorf("Init: expected 2 taps, got %d", len(m.Taps))
	}

	// --- Phase 2: Push ---
	// Push manifest to a "remote" temp directory via FileBackend
	remoteDir := t.TempDir()
	remotePath := filepath.Join(remoteDir, "brew-sync.toml")
	backend := sync.NewFileBackend(remotePath)

	if err := backend.Push(localManifestPath); err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	// Verify remote file exists
	if _, err := os.Stat(remotePath); err != nil {
		t.Fatalf("Remote manifest not found after push: %v", err)
	}

	// --- Phase 3: Pull ---
	// Pull manifest to a different "machine" temp directory
	pullDir := t.TempDir()
	pulledManifestPath := filepath.Join(pullDir, "brew-sync.toml")

	if err := backend.Pull(pulledManifestPath); err != nil {
		t.Fatalf("Pull failed: %v", err)
	}

	// Verify pulled manifest matches original
	pulledManifest, err := manager.Load(pulledManifestPath)
	if err != nil {
		t.Fatalf("Load pulled manifest failed: %v", err)
	}
	if len(pulledManifest.Formulae) != 3 {
		t.Errorf("Pull: expected 3 formulae, got %d", len(pulledManifest.Formulae))
	}
	if len(pulledManifest.Casks) != 1 {
		t.Errorf("Pull: expected 1 cask, got %d", len(pulledManifest.Casks))
	}

	// --- Phase 4: Status ---
	// Simulate a different local state on the "second machine"
	// This machine has git (same), go 1.22 (older), and wget (extra), but no curl or firefox
	secondMock := brew.NewMockBrewRunner()
	secondMock.Formulae = []diff.Package{
		{Name: "git", Version: "2.40"},
		{Name: "go", Version: "1.22"},
		{Name: "wget", Version: "1.21"},
	}
	secondMock.Casks = []diff.Package{}

	secondFormulae, _ := secondMock.ListFormulae()
	secondCasks, _ := secondMock.ListCasks()

	localState := &diff.LocalState{
		Formulae: secondFormulae,
		Casks:    secondCasks,
	}

	result := diff.ComputeDiff(pulledManifest, localState, "")

	// Verify diff results:
	// ToInstall: curl (in manifest, not local), firefox (cask in manifest, not local)
	// ToUpgrade: go (manifest=1.23, local=1.22)
	// ToRemove: wget (local only)
	// Unchanged: git (same version)
	if len(result.ToInstall) != 2 {
		t.Errorf("Status: expected 2 to install, got %d: %v", len(result.ToInstall), packageNames(result.ToInstall))
	}
	if len(result.ToUpgrade) != 1 {
		t.Errorf("Status: expected 1 to upgrade, got %d: %v", len(result.ToUpgrade), packageNames(result.ToUpgrade))
	}
	if len(result.ToRemove) != 1 {
		t.Errorf("Status: expected 1 to remove, got %d: %v", len(result.ToRemove), packageNames(result.ToRemove))
	}
	if len(result.Unchanged) != 1 {
		t.Errorf("Status: expected 1 unchanged, got %d: %v", len(result.Unchanged), packageNames(result.Unchanged))
	}

	// --- Phase 5: Apply ---
	// Apply the diff using the second mock runner
	// By default, removals are skipped (skipRemove=true)
	report, applyErr := diff.ApplyDiff(result, secondMock, false, diff.ApplyOptions{SkipRemove: false})
	if applyErr != nil {
		t.Fatalf("Apply failed: %v", applyErr)
	}

	// Verify correct operations were performed
	if secondMock.InstallCalls != 2 {
		t.Errorf("Apply: expected 2 install calls, got %d", secondMock.InstallCalls)
	}
	if secondMock.UpgradeCalls != 1 {
		t.Errorf("Apply: expected 1 upgrade call, got %d", secondMock.UpgradeCalls)
	}
	if secondMock.UninstallCalls != 1 {
		t.Errorf("Apply: expected 1 uninstall call, got %d", secondMock.UninstallCalls)
	}
	if report.ErrorCount != 0 {
		t.Errorf("Apply: expected 0 errors, got %d", report.ErrorCount)
	}

	// Verify the specific operations in the call log
	callOps := make(map[string][]string)
	for _, call := range secondMock.Calls {
		callOps[call.Operation] = append(callOps[call.Operation], call.Package)
	}
	if !containsStr(callOps["install"], "curl") {
		t.Error("Apply: expected install call for 'curl'")
	}
	if !containsStr(callOps["install"], "firefox") {
		t.Error("Apply: expected install call for 'firefox'")
	}
	if !containsStr(callOps["upgrade"], "go") {
		t.Error("Apply: expected upgrade call for 'go'")
	}
	if !containsStr(callOps["uninstall"], "wget") {
		t.Error("Apply: expected uninstall call for 'wget'")
	}
}

// ---------------------------------------------------------------------------
// Integration Test: Apply with default skip-remove behavior
// Validates: Local-only packages are not removed by default
// ---------------------------------------------------------------------------

func TestIntegration_ApplySkipRemoveDefault(t *testing.T) {
	manager := manifest.NewManifestManager()

	// Create a manifest with some packages
	m := &manifest.Manifest{
		Version: 1,
		Formulae: []manifest.PackageEntry{
			{Name: "git", Version: "2.40"},
		},
	}

	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "brew-sync.toml")
	if err := manager.Save(manifestPath, m); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := manager.Load(manifestPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Local state has git (in manifest) + wget (local-only)
	mock := brew.NewMockBrewRunner()
	mock.Formulae = []diff.Package{
		{Name: "git", Version: "2.40"},
		{Name: "wget", Version: "1.0"},
	}
	mock.Casks = []diff.Package{}

	formulae, _ := mock.ListFormulae()
	casks, _ := mock.ListCasks()

	localState := &diff.LocalState{
		Formulae: formulae,
		Casks:    casks,
	}

	result := diff.ComputeDiff(loaded, localState, "")

	// Apply with SkipRemove=true (the default in the CLI)
	report, err := diff.ApplyDiff(result, mock, false, diff.ApplyOptions{SkipRemove: true})
	if err != nil {
		t.Fatalf("ApplyDiff returned error: %v", err)
	}

	// wget should NOT be uninstalled
	if mock.UninstallCalls != 0 {
		t.Errorf("expected 0 uninstall calls with SkipRemove, got %d", mock.UninstallCalls)
	}

	// Report should indicate skipped removals
	if report.SkippedRemoveCount != 1 {
		t.Errorf("expected SkippedRemoveCount=1, got %d", report.SkippedRemoveCount)
	}
}

// ---------------------------------------------------------------------------
// Integration Test: Apply with --dry-run end-to-end
// Validates: Requirements 5.1, 5.2
// ---------------------------------------------------------------------------

func TestIntegration_ApplyDryRun(t *testing.T) {
	manager := manifest.NewManifestManager()

	// Create a manifest with some packages
	m := &manifest.Manifest{
		Version: 1,
		Formulae: []manifest.PackageEntry{
			{Name: "git", Version: "2.40"},
			{Name: "go", Version: "1.24"},
			{Name: "ripgrep"},
		},
		Casks: []manifest.PackageEntry{
			{Name: "firefox"},
		},
		Taps: []string{"homebrew/core"},
	}

	// Save and reload to simulate real workflow
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "brew-sync.toml")
	if err := manager.Save(manifestPath, m); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := manager.Load(manifestPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Set up local state with differences
	mock := brew.NewMockBrewRunner()
	mock.Formulae = []diff.Package{
		{Name: "git", Version: "2.40"}, // same → unchanged
		{Name: "go", Version: "1.23"},  // older → upgrade
		{Name: "wget", Version: "1.0"}, // extra → remove
	}
	mock.Casks = []diff.Package{} // firefox missing → install

	formulae, _ := mock.ListFormulae()
	casks, _ := mock.ListCasks()

	localState := &diff.LocalState{
		Formulae: formulae,
		Casks:    casks,
	}

	result := diff.ComputeDiff(loaded, localState, "")

	// Apply with dry-run
	report, err := diff.ApplyDiff(result, mock, true)
	if err != nil {
		t.Fatalf("ApplyDiff dry-run returned error: %v", err)
	}

	// Verify report has correct planned counts
	if !report.Planned {
		t.Error("DryRun: expected Planned=true")
	}
	if report.InstallCount != 2 {
		t.Errorf("DryRun: expected InstallCount=2, got %d", report.InstallCount)
	}
	if report.UpgradeCount != 1 {
		t.Errorf("DryRun: expected UpgradeCount=1, got %d", report.UpgradeCount)
	}
	if report.RemoveCount != 1 {
		t.Errorf("DryRun: expected RemoveCount=1, got %d", report.RemoveCount)
	}

	// Verify MockBrewRunner received ZERO mutation calls
	if mock.InstallCalls != 0 {
		t.Errorf("DryRun: expected 0 install calls, got %d", mock.InstallCalls)
	}
	if mock.UninstallCalls != 0 {
		t.Errorf("DryRun: expected 0 uninstall calls, got %d", mock.UninstallCalls)
	}
	if mock.UpgradeCalls != 0 {
		t.Errorf("DryRun: expected 0 upgrade calls, got %d", mock.UpgradeCalls)
	}
}

// ---------------------------------------------------------------------------
// Integration Test: Error path — missing manifest
// Validates: Requirement 10.1
// ---------------------------------------------------------------------------

func TestIntegration_ErrorMissingManifest(t *testing.T) {
	manager := manifest.NewManifestManager()

	// Try to load a manifest from a nonexistent path
	nonexistentPath := filepath.Join(t.TempDir(), "does-not-exist", "brew-sync.toml")
	_, err := manager.Load(nonexistentPath)

	if err == nil {
		t.Fatal("expected error when loading nonexistent manifest, got nil")
	}

	if !os.IsNotExist(err) {
		t.Errorf("expected os.IsNotExist(err) to be true, got false; error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Integration Test: Error path — unreachable backend
// Validates: Requirement 7.1
// ---------------------------------------------------------------------------

func TestIntegration_ErrorUnreachableBackend(t *testing.T) {
	// Create a FileBackend pointing to a nonexistent remote path
	backend := sync.NewFileBackend("/nonexistent/remote/path/brew-sync.toml")

	destDir := t.TempDir()
	destFile := filepath.Join(destDir, "pulled.toml")

	err := backend.Pull(destFile)
	if err == nil {
		t.Fatal("expected error when pulling from unreachable backend, got nil")
	}

	// Verify the error is descriptive
	errMsg := err.Error()
	if !strings.Contains(errMsg, "not found") {
		t.Errorf("expected error to mention 'not found', got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "/nonexistent/remote/path/brew-sync.toml") {
		t.Errorf("expected error to mention the path, got: %s", errMsg)
	}
}

// ---------------------------------------------------------------------------
// Integration Test: Config loading and machine tag
// Validates: Requirements 9.1, 12.1
// ---------------------------------------------------------------------------

func TestIntegration_ConfigLoadingAndMachineTag(t *testing.T) {
	// Create a config TOML file
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "config.toml")

	configContent := `manifest_path = "/custom/path/brew-sync.toml"
machine_tag = "work-laptop"
sync_backend = "file"

[file]
remote_path = "/shared/brew-sync.toml"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Load config using the cmd package's loadConfig
	cfg, err := loadConfig(configPath)
	if err != nil {
		t.Fatalf("loadConfig failed: %v", err)
	}

	// Verify getManifestPath returns the configured path
	manifestPath := getManifestPath(cfg)
	if manifestPath != "/custom/path/brew-sync.toml" {
		t.Errorf("getManifestPath: got %q, want %q", manifestPath, "/custom/path/brew-sync.toml")
	}

	// Verify getMachineTag returns the configured tag
	machineTag := getMachineTag(cfg)
	if machineTag != "work-laptop" {
		t.Errorf("getMachineTag: got %q, want %q", machineTag, "work-laptop")
	}

	// Verify defaults when config is nil
	if got := getManifestPath(nil); got != defaultManifestPath {
		t.Errorf("getManifestPath(nil): got %q, want %q", got, defaultManifestPath)
	}
	if got := getMachineTag(nil); got != "" {
		t.Errorf("getMachineTag(nil): got %q, want %q", got, "")
	}

	// Verify defaults when config has empty values
	emptyCfg := &manifest.Manifest{}
	_ = emptyCfg // just to show we're testing the zero-value path
	emptyConfig := loadConfigGraceful()
	if got := getManifestPath(emptyConfig); got != defaultManifestPath {
		t.Errorf("getManifestPath(empty): got %q, want %q", got, defaultManifestPath)
	}
}

// ---------------------------------------------------------------------------
// Integration Test: Machine tag filtering in workflow
// Validates: Requirements 2.7, 3.1, 3.2, 3.3
// ---------------------------------------------------------------------------

func TestIntegration_MachineTagFiltering(t *testing.T) {
	manager := manifest.NewManifestManager()

	// Create a manifest with packages that have machine filters
	m := &manifest.Manifest{
		Version: 1,
		Formulae: []manifest.PackageEntry{
			{Name: "git"}, // no filter → always included
			{Name: "docker", OnlyOn: []string{"work-laptop"}},        // only on work-laptop
			{Name: "gaming-tools", OnlyOn: []string{"home-desktop"}}, // only on home-desktop
		},
		Casks: []manifest.PackageEntry{
			{Name: "firefox"}, // no filter → always included
			{Name: "slack", ExceptOn: []string{"home-desktop"}}, // excluded on home-desktop
			{Name: "steam", ExceptOn: []string{"work-laptop"}},  // excluded on work-laptop
		},
		Taps: []string{"homebrew/core"},
	}

	// Save and reload
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "brew-sync.toml")
	if err := manager.Save(manifestPath, m); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	loaded, err := manager.Load(manifestPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Test with machine tag "work-laptop"
	t.Run("work-laptop", func(t *testing.T) {
		localState := &diff.LocalState{} // empty local state

		result := diff.ComputeDiff(loaded, localState, "work-laptop")

		// Expected ToInstall: git, docker, firefox, slack (not gaming-tools, not steam)
		installNames := packageNames(result.ToInstall)
		if len(result.ToInstall) != 4 {
			t.Errorf("work-laptop: expected 4 to install, got %d: %v", len(result.ToInstall), installNames)
		}
		if !containsStr(installNames, "git") {
			t.Error("work-laptop: expected 'git' in ToInstall")
		}
		if !containsStr(installNames, "docker") {
			t.Error("work-laptop: expected 'docker' in ToInstall (only_on matches)")
		}
		if !containsStr(installNames, "firefox") {
			t.Error("work-laptop: expected 'firefox' in ToInstall")
		}
		if !containsStr(installNames, "slack") {
			t.Error("work-laptop: expected 'slack' in ToInstall (except_on doesn't match)")
		}
		if containsStr(installNames, "gaming-tools") {
			t.Error("work-laptop: 'gaming-tools' should be excluded (only_on=home-desktop)")
		}
		if containsStr(installNames, "steam") {
			t.Error("work-laptop: 'steam' should be excluded (except_on=work-laptop)")
		}
	})

	// Test with machine tag "home-desktop"
	t.Run("home-desktop", func(t *testing.T) {
		localState := &diff.LocalState{} // empty local state

		result := diff.ComputeDiff(loaded, localState, "home-desktop")

		// Expected ToInstall: git, gaming-tools, firefox, steam (not docker, not slack)
		installNames := packageNames(result.ToInstall)
		if len(result.ToInstall) != 4 {
			t.Errorf("home-desktop: expected 4 to install, got %d: %v", len(result.ToInstall), installNames)
		}
		if !containsStr(installNames, "git") {
			t.Error("home-desktop: expected 'git' in ToInstall")
		}
		if !containsStr(installNames, "gaming-tools") {
			t.Error("home-desktop: expected 'gaming-tools' in ToInstall (only_on matches)")
		}
		if !containsStr(installNames, "firefox") {
			t.Error("home-desktop: expected 'firefox' in ToInstall")
		}
		if !containsStr(installNames, "steam") {
			t.Error("home-desktop: expected 'steam' in ToInstall (except_on doesn't match)")
		}
		if containsStr(installNames, "docker") {
			t.Error("home-desktop: 'docker' should be excluded (only_on=work-laptop)")
		}
		if containsStr(installNames, "slack") {
			t.Error("home-desktop: 'slack' should be excluded (except_on=home-desktop)")
		}
	})

	// Test with machine tag "work-laptop" and apply to verify correct packages are installed
	t.Run("work-laptop-apply", func(t *testing.T) {
		mock := brew.NewMockBrewRunner()
		mock.Formulae = []diff.Package{}
		mock.Casks = []diff.Package{}

		localState := &diff.LocalState{}
		result := diff.ComputeDiff(loaded, localState, "work-laptop")

		report, err := diff.ApplyDiff(result, mock, false)
		if err != nil {
			t.Fatalf("Apply failed: %v", err)
		}

		if mock.InstallCalls != 4 {
			t.Errorf("expected 4 install calls, got %d", mock.InstallCalls)
		}
		if report.ErrorCount != 0 {
			t.Errorf("expected 0 errors, got %d", report.ErrorCount)
		}

		// Verify the specific packages installed
		installedPkgs := make([]string, 0)
		for _, call := range mock.Calls {
			if call.Operation == "install" {
				installedPkgs = append(installedPkgs, call.Package)
			}
		}
		if containsStr(installedPkgs, "gaming-tools") {
			t.Error("gaming-tools should not be installed on work-laptop")
		}
		if containsStr(installedPkgs, "steam") {
			t.Error("steam should not be installed on work-laptop")
		}
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// packageNames extracts names from a slice of PackageEntry.
func packageNames(entries []manifest.PackageEntry) []string {
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name
	}
	return names
}

// containsStr checks if a string slice contains a given value.
func containsStr(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}
