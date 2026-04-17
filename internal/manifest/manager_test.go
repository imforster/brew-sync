package manifest

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Load / Save tests
// Requirements: 8.1, 8.2
// ---------------------------------------------------------------------------

func TestLoadSave_ValidManifest(t *testing.T) {
	m := &Manifest{
		Version: 1,
		Metadata: ManifestMetadata{
			UpdatedAt: "2025-01-15T10:30:00Z",
			UpdatedBy: "alice",
			Machine:   "alice-macbook",
		},
		Formulae: []PackageEntry{
			{Name: "git", Version: "2.40"},
			{Name: "go", Version: "1.23", OnlyOn: []string{"work-laptop"}},
		},
		Casks: []PackageEntry{
			{Name: "firefox"},
			{Name: "slack", ExceptOn: []string{"home-desktop"}},
		},
		Taps: []string{"homebrew/cask-fonts", "hashicorp/tap"},
	}

	mgr := NewManifestManager()
	dir := t.TempDir()
	path := filepath.Join(dir, "brew-sync.toml")

	if err := mgr.Save(path, m); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := mgr.Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Verify all fields match
	if loaded.Version != m.Version {
		t.Errorf("Version: got %d, want %d", loaded.Version, m.Version)
	}
	if loaded.Metadata.UpdatedAt != m.Metadata.UpdatedAt {
		t.Errorf("UpdatedAt: got %q, want %q", loaded.Metadata.UpdatedAt, m.Metadata.UpdatedAt)
	}
	if loaded.Metadata.UpdatedBy != m.Metadata.UpdatedBy {
		t.Errorf("UpdatedBy: got %q, want %q", loaded.Metadata.UpdatedBy, m.Metadata.UpdatedBy)
	}
	if loaded.Metadata.Machine != m.Metadata.Machine {
		t.Errorf("Machine: got %q, want %q", loaded.Metadata.Machine, m.Metadata.Machine)
	}
	if len(loaded.Formulae) != len(m.Formulae) {
		t.Fatalf("Formulae count: got %d, want %d", len(loaded.Formulae), len(m.Formulae))
	}
	for i, f := range loaded.Formulae {
		if f.Name != m.Formulae[i].Name {
			t.Errorf("Formulae[%d].Name: got %q, want %q", i, f.Name, m.Formulae[i].Name)
		}
		if f.Version != m.Formulae[i].Version {
			t.Errorf("Formulae[%d].Version: got %q, want %q", i, f.Version, m.Formulae[i].Version)
		}
	}
	if len(loaded.Casks) != len(m.Casks) {
		t.Fatalf("Casks count: got %d, want %d", len(loaded.Casks), len(m.Casks))
	}
	for i, c := range loaded.Casks {
		if c.Name != m.Casks[i].Name {
			t.Errorf("Casks[%d].Name: got %q, want %q", i, c.Name, m.Casks[i].Name)
		}
	}
	if len(loaded.Taps) != len(m.Taps) {
		t.Fatalf("Taps count: got %d, want %d", len(loaded.Taps), len(m.Taps))
	}
	for i, tap := range loaded.Taps {
		if tap != m.Taps[i] {
			t.Errorf("Taps[%d]: got %q, want %q", i, tap, m.Taps[i])
		}
	}
}

func TestLoad_NonexistentFile(t *testing.T) {
	mgr := NewManifestManager()
	_, err := mgr.Load("/nonexistent/path/brew-sync.toml")
	if err == nil {
		t.Fatal("expected error loading nonexistent file, got nil")
	}
	if !os.IsNotExist(err) {
		t.Errorf("expected os.IsNotExist error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Validate tests
// Requirements: 8.3, 8.4, 8.5, 8.6, 8.7, 8.8
// ---------------------------------------------------------------------------

func TestValidate_ValidManifest(t *testing.T) {
	m := &Manifest{
		Version: 1,
		Metadata: ManifestMetadata{
			UpdatedAt: "2025-01-15T10:30:00Z",
		},
		Formulae: []PackageEntry{
			{Name: "git"},
			{Name: "go", Version: "1.23"},
		},
		Casks: []PackageEntry{
			{Name: "firefox"},
		},
		Taps: []string{"homebrew/core", "hashicorp/tap"},
	}

	mgr := NewManifestManager()
	if err := mgr.Validate(m); err != nil {
		t.Errorf("expected valid manifest to pass validation, got: %v", err)
	}
}

func TestValidate_UnsupportedVersion(t *testing.T) {
	m := &Manifest{
		Version:  2,
		Formulae: []PackageEntry{{Name: "git"}},
		Taps:     []string{"owner/repo"},
	}

	mgr := NewManifestManager()
	err := mgr.Validate(m)
	if err == nil {
		t.Fatal("expected error for unsupported version, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported manifest version") {
		t.Errorf("expected error about unsupported version, got: %v", err)
	}
}

func TestValidate_DuplicateFormulae(t *testing.T) {
	m := &Manifest{
		Version: 1,
		Formulae: []PackageEntry{
			{Name: "git"},
			{Name: "git"},
		},
		Taps: []string{"owner/repo"},
	}

	mgr := NewManifestManager()
	err := mgr.Validate(m)
	if err == nil {
		t.Fatal("expected error for duplicate formulae, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate formula: git") {
		t.Errorf("expected error about duplicate formula, got: %v", err)
	}
}

func TestValidate_DuplicateCasks(t *testing.T) {
	m := &Manifest{
		Version: 1,
		Casks: []PackageEntry{
			{Name: "firefox"},
			{Name: "firefox"},
		},
		Taps: []string{"owner/repo"},
	}

	mgr := NewManifestManager()
	err := mgr.Validate(m)
	if err == nil {
		t.Fatal("expected error for duplicate casks, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate cask: firefox") {
		t.Errorf("expected error about duplicate cask, got: %v", err)
	}
}

func TestValidate_EmptyFormulaName(t *testing.T) {
	m := &Manifest{
		Version: 1,
		Formulae: []PackageEntry{
			{Name: ""},
		},
		Taps: []string{"owner/repo"},
	}

	mgr := NewManifestManager()
	err := mgr.Validate(m)
	if err == nil {
		t.Fatal("expected error for empty formula name, got nil")
	}
	if !strings.Contains(err.Error(), "formula entry has empty name") {
		t.Errorf("expected error about empty formula name, got: %v", err)
	}
}

func TestValidate_EmptyCaskName(t *testing.T) {
	m := &Manifest{
		Version: 1,
		Casks: []PackageEntry{
			{Name: ""},
		},
		Taps: []string{"owner/repo"},
	}

	mgr := NewManifestManager()
	err := mgr.Validate(m)
	if err == nil {
		t.Fatal("expected error for empty cask name, got nil")
	}
	if !strings.Contains(err.Error(), "cask entry has empty name") {
		t.Errorf("expected error about empty cask name, got: %v", err)
	}
}

func TestValidate_BothOnlyOnAndExceptOn(t *testing.T) {
	m := &Manifest{
		Version: 1,
		Formulae: []PackageEntry{
			{Name: "docker", OnlyOn: []string{"work"}, ExceptOn: []string{"home"}},
		},
		Taps: []string{"owner/repo"},
	}

	mgr := NewManifestManager()
	err := mgr.Validate(m)
	if err == nil {
		t.Fatal("expected error for both only_on and except_on, got nil")
	}
	if !strings.Contains(err.Error(), "both only_on and except_on") {
		t.Errorf("expected error about mutual exclusivity, got: %v", err)
	}
}

func TestValidate_InvalidTapFormat(t *testing.T) {
	cases := []struct {
		name string
		tap  string
	}{
		{"no slash", "justname"},
		{"empty owner", "/repo"},
		{"empty repo", "owner/"},
		{"too many slashes", "a/b/c"},
		{"empty string", ""},
	}

	mgr := NewManifestManager()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &Manifest{
				Version: 1,
				Taps:    []string{tc.tap},
			}
			err := mgr.Validate(m)
			if err == nil {
				t.Fatalf("expected error for invalid tap %q, got nil", tc.tap)
			}
			if !strings.Contains(err.Error(), "invalid tap format") {
				t.Errorf("expected error about invalid tap format, got: %v", err)
			}
		})
	}
}

func TestValidate_MultipleErrors(t *testing.T) {
	m := &Manifest{
		Version: 2, // unsupported version
		Formulae: []PackageEntry{
			{Name: "git"},
			{Name: "git"}, // duplicate
		},
		Casks: []PackageEntry{
			{Name: ""}, // empty name
		},
		Taps: []string{"badtap"}, // invalid format
	}

	mgr := NewManifestManager()
	err := mgr.Validate(m)
	if err == nil {
		t.Fatal("expected multiple errors, got nil")
	}

	errStr := err.Error()
	expectedSubstrings := []string{
		"unsupported manifest version",
		"duplicate formula: git",
		"cask entry has empty name",
		"invalid tap format",
	}
	for _, sub := range expectedSubstrings {
		if !strings.Contains(errStr, sub) {
			t.Errorf("expected error to contain %q, got: %v", sub, errStr)
		}
	}
}

// ---------------------------------------------------------------------------
// BuildFromLocal tests
// Requirements: 1.2, 1.3
// ---------------------------------------------------------------------------

func TestBuildFromLocal_Empty(t *testing.T) {
	mgr := NewManifestManager()
	m := mgr.BuildFromLocal(nil, nil, nil)

	if m.Version != 1 {
		t.Errorf("Version: got %d, want 1", m.Version)
	}
	if m.Metadata.UpdatedAt == "" {
		t.Error("expected UpdatedAt to be set")
	}
	// Verify UpdatedAt is a valid RFC3339 timestamp
	if _, err := time.Parse(time.RFC3339, m.Metadata.UpdatedAt); err != nil {
		t.Errorf("UpdatedAt is not valid RFC3339: %v", err)
	}
	if len(m.Formulae) != 0 {
		t.Errorf("Formulae: got %d entries, want 0", len(m.Formulae))
	}
	if len(m.Casks) != 0 {
		t.Errorf("Casks: got %d entries, want 0", len(m.Casks))
	}
	if len(m.Taps) != 0 {
		t.Errorf("Taps: got %d entries, want 0", len(m.Taps))
	}

	// A manifest built from empty inputs should be valid
	if err := mgr.Validate(m); err != nil {
		t.Errorf("expected empty manifest to be valid, got: %v", err)
	}
}

func TestBuildFromLocal_Populated(t *testing.T) {
	mgr := NewManifestManager()

	formulae := []LocalPackage{
		{Name: "go", Version: "1.23"},
		{Name: "git", Version: "2.40"},
	}
	casks := []LocalPackage{
		{Name: "slack", Version: "4.0"},
		{Name: "firefox", Version: "120"},
	}
	taps := []string{"hashicorp/tap", "homebrew/core"}

	before := time.Now().UTC()
	m := mgr.BuildFromLocal(formulae, casks, taps)
	after := time.Now().UTC()

	// Version
	if m.Version != 1 {
		t.Errorf("Version: got %d, want 1", m.Version)
	}

	// UpdatedAt should be between before and after
	updatedAt, err := time.Parse(time.RFC3339, m.Metadata.UpdatedAt)
	if err != nil {
		t.Fatalf("UpdatedAt is not valid RFC3339: %v", err)
	}
	if updatedAt.Before(before.Truncate(time.Second)) || updatedAt.After(after.Add(time.Second)) {
		t.Errorf("UpdatedAt %v not in expected range [%v, %v]", updatedAt, before, after)
	}

	// Formulae sorted by name
	if len(m.Formulae) != 2 {
		t.Fatalf("Formulae count: got %d, want 2", len(m.Formulae))
	}
	if m.Formulae[0].Name != "git" || m.Formulae[1].Name != "go" {
		t.Errorf("Formulae not sorted: got [%s, %s], want [git, go]", m.Formulae[0].Name, m.Formulae[1].Name)
	}

	// Casks sorted by name
	if len(m.Casks) != 2 {
		t.Fatalf("Casks count: got %d, want 2", len(m.Casks))
	}
	if m.Casks[0].Name != "firefox" || m.Casks[1].Name != "slack" {
		t.Errorf("Casks not sorted: got [%s, %s], want [firefox, slack]", m.Casks[0].Name, m.Casks[1].Name)
	}

	// Taps sorted
	if len(m.Taps) != 2 {
		t.Fatalf("Taps count: got %d, want 2", len(m.Taps))
	}
	if m.Taps[0] != "hashicorp/tap" || m.Taps[1] != "homebrew/core" {
		t.Errorf("Taps not sorted: got %v, want [hashicorp/tap, homebrew/core]", m.Taps)
	}

	// No machine filters set
	for i, f := range m.Formulae {
		if len(f.OnlyOn) != 0 {
			t.Errorf("Formulae[%d].OnlyOn should be empty", i)
		}
		if len(f.ExceptOn) != 0 {
			t.Errorf("Formulae[%d].ExceptOn should be empty", i)
		}
	}
	for i, c := range m.Casks {
		if len(c.OnlyOn) != 0 {
			t.Errorf("Casks[%d].OnlyOn should be empty", i)
		}
		if len(c.ExceptOn) != 0 {
			t.Errorf("Casks[%d].ExceptOn should be empty", i)
		}
	}
}

func TestBuildFromLocal_SortsEntries(t *testing.T) {
	mgr := NewManifestManager()

	// Provide entries in reverse alphabetical order
	formulae := []LocalPackage{
		{Name: "zsh"},
		{Name: "node"},
		{Name: "curl"},
		{Name: "awk"},
	}
	casks := []LocalPackage{
		{Name: "zoom"},
		{Name: "docker"},
		{Name: "alacritty"},
	}
	taps := []string{"z-org/z-repo", "a-org/a-repo", "m-org/m-repo"}

	m := mgr.BuildFromLocal(formulae, casks, taps)

	// Verify formulae are sorted
	formulaeNames := make([]string, len(m.Formulae))
	for i, f := range m.Formulae {
		formulaeNames[i] = f.Name
	}
	if !sort.StringsAreSorted(formulaeNames) {
		t.Errorf("Formulae not sorted: %v", formulaeNames)
	}

	// Verify casks are sorted
	caskNames := make([]string, len(m.Casks))
	for i, c := range m.Casks {
		caskNames[i] = c.Name
	}
	if !sort.StringsAreSorted(caskNames) {
		t.Errorf("Casks not sorted: %v", caskNames)
	}

	// Verify taps are sorted
	if !sort.StringsAreSorted(m.Taps) {
		t.Errorf("Taps not sorted: %v", m.Taps)
	}
}
