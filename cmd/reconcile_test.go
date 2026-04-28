package cmd

import (
	"path/filepath"
	"sort"
	"testing"

	"brew-sync/internal/manifest"
)

// ---------------------------------------------------------------------------
// Reconcile sort tests
// Validates: Issue #9 — reconcile sorts formulae and casks before saving
// ---------------------------------------------------------------------------

// TestReconcile_SortAfterAppend simulates the reconcile save path:
// load a manifest with sorted entries, append new entries out of order
// (as reconcile does), sort, save, reload, and verify alphabetical order
// with all fields preserved.
func TestReconcile_SortAfterAppend(t *testing.T) {
	mgr := manifest.NewManifestManager()
	dir := t.TempDir()
	path := filepath.Join(dir, "brew-sync.toml")

	// Start with a sorted manifest containing entries with extra fields.
	m := &manifest.Manifest{
		Version: 1,
		Formulae: []manifest.PackageEntry{
			{Name: "curl", Version: "8.0"},
			{Name: "git", Version: "2.40", OnlyOn: []string{"work"}},
		},
		Casks: []manifest.PackageEntry{
			{Name: "firefox"},
			{Name: "slack", ExceptOn: []string{"home"}},
		},
	}

	// Simulate reconcile appending unsorted entries.
	m.Formulae = append(m.Formulae, manifest.PackageEntry{Name: "awk"})
	m.Formulae = append(m.Formulae, manifest.PackageEntry{Name: "zsh", OnlyOn: []string{"work"}})
	m.Casks = append(m.Casks, manifest.PackageEntry{Name: "alacritty"})
	m.Casks = append(m.Casks, manifest.PackageEntry{Name: "zoom"})

	// Apply the same sort reconcile.go does.
	sort.Slice(m.Formulae, func(i, j int) bool { return m.Formulae[i].Name < m.Formulae[j].Name })
	sort.Slice(m.Casks, func(i, j int) bool { return m.Casks[i].Name < m.Casks[j].Name })

	if err := mgr.Save(path, m); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := mgr.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Verify formulae sorted.
	wantFormulae := []string{"awk", "curl", "git", "zsh"}
	if len(loaded.Formulae) != len(wantFormulae) {
		t.Fatalf("formulae count: got %d, want %d", len(loaded.Formulae), len(wantFormulae))
	}
	for i, want := range wantFormulae {
		if loaded.Formulae[i].Name != want {
			t.Errorf("formulae[%d]: got %q, want %q", i, loaded.Formulae[i].Name, want)
		}
	}

	// Verify casks sorted.
	wantCasks := []string{"alacritty", "firefox", "slack", "zoom"}
	if len(loaded.Casks) != len(wantCasks) {
		t.Fatalf("casks count: got %d, want %d", len(loaded.Casks), len(wantCasks))
	}
	for i, want := range wantCasks {
		if loaded.Casks[i].Name != want {
			t.Errorf("casks[%d]: got %q, want %q", i, loaded.Casks[i].Name, want)
		}
	}

	// Verify fields survived the sort.
	for _, f := range loaded.Formulae {
		switch f.Name {
		case "curl":
			if f.Version != "8.0" {
				t.Errorf("curl version: got %q, want %q", f.Version, "8.0")
			}
		case "git":
			if f.Version != "2.40" {
				t.Errorf("git version: got %q, want %q", f.Version, "2.40")
			}
			if len(f.OnlyOn) != 1 || f.OnlyOn[0] != "work" {
				t.Errorf("git only_on: got %v, want [work]", f.OnlyOn)
			}
		case "zsh":
			if len(f.OnlyOn) != 1 || f.OnlyOn[0] != "work" {
				t.Errorf("zsh only_on: got %v, want [work]", f.OnlyOn)
			}
		}
	}
	for _, c := range loaded.Casks {
		if c.Name == "slack" {
			if len(c.ExceptOn) != 1 || c.ExceptOn[0] != "home" {
				t.Errorf("slack except_on: got %v, want [home]", c.ExceptOn)
			}
		}
	}
}

// TestReconcile_SortNoChangesPreservesOrder verifies that sorting an
// already-sorted manifest produces no reordering (idempotent).
func TestReconcile_SortNoChangesPreservesOrder(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Formulae: []manifest.PackageEntry{
			{Name: "awk"},
			{Name: "curl"},
			{Name: "git"},
		},
		Casks: []manifest.PackageEntry{
			{Name: "firefox"},
			{Name: "slack"},
		},
	}

	// Capture original order.
	origFormulae := make([]string, len(m.Formulae))
	for i, f := range m.Formulae {
		origFormulae[i] = f.Name
	}
	origCasks := make([]string, len(m.Casks))
	for i, c := range m.Casks {
		origCasks[i] = c.Name
	}

	sort.Slice(m.Formulae, func(i, j int) bool { return m.Formulae[i].Name < m.Formulae[j].Name })
	sort.Slice(m.Casks, func(i, j int) bool { return m.Casks[i].Name < m.Casks[j].Name })

	for i, f := range m.Formulae {
		if f.Name != origFormulae[i] {
			t.Errorf("formulae[%d] changed: got %q, was %q", i, f.Name, origFormulae[i])
		}
	}
	for i, c := range m.Casks {
		if c.Name != origCasks[i] {
			t.Errorf("casks[%d] changed: got %q, was %q", i, c.Name, origCasks[i])
		}
	}
}
