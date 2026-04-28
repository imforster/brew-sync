package diff

import (
	"testing"

	"brew-sync/internal/manifest"
)

// helper to find a package by name in a slice of PackageEntry.
func findEntry(entries []manifest.PackageEntry, name string) (manifest.PackageEntry, bool) {
	for _, e := range entries {
		if e.Name == name {
			return e, true
		}
	}
	return manifest.PackageEntry{}, false
}

// entryNames returns a sorted list of names from a slice of PackageEntry.
func entryNames(entries []manifest.PackageEntry) []string {
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name
	}
	return names
}

// ---------------------------------------------------------------------------
// ComputeDiff unit tests
// ---------------------------------------------------------------------------

// TestComputeDiff_EmptySets verifies that an empty manifest and empty local
// state produce a completely empty diff result.
// Validates: Requirements 2.1, 2.2
func TestComputeDiff_EmptySets(t *testing.T) {
	m := &manifest.Manifest{Version: 1}
	local := &LocalState{}

	result := ComputeDiff(m, local, "my-machine")

	if len(result.ToInstall) != 0 {
		t.Errorf("ToInstall: got %d, want 0", len(result.ToInstall))
	}
	if len(result.ToRemove) != 0 {
		t.Errorf("ToRemove: got %d, want 0", len(result.ToRemove))
	}
	if len(result.ToUpgrade) != 0 {
		t.Errorf("ToUpgrade: got %d, want 0", len(result.ToUpgrade))
	}
	if len(result.Unchanged) != 0 {
		t.Errorf("Unchanged: got %d, want 0", len(result.Unchanged))
	}
}

// TestComputeDiff_IdenticalSets verifies that when the manifest and local
// state contain the same packages, all are classified as Unchanged.
// Validates: Requirements 2.2, 2.6
func TestComputeDiff_IdenticalSets(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Formulae: []manifest.PackageEntry{
			{Name: "git", Version: "2.40"},
			{Name: "go", Version: "1.23"},
		},
		Casks: []manifest.PackageEntry{
			{Name: "firefox"},
		},
	}
	local := &LocalState{
		Formulae: []Package{
			{Name: "git", Version: "2.40"},
			{Name: "go", Version: "1.23"},
		},
		Casks: []Package{
			{Name: "firefox", Version: "120.0"},
		},
	}

	result := ComputeDiff(m, local, "my-machine")

	if len(result.ToInstall) != 0 {
		t.Errorf("ToInstall: got %v, want empty", entryNames(result.ToInstall))
	}
	if len(result.ToRemove) != 0 {
		t.Errorf("ToRemove: got %v, want empty", entryNames(result.ToRemove))
	}
	if len(result.ToUpgrade) != 0 {
		t.Errorf("ToUpgrade: got %v, want empty", entryNames(result.ToUpgrade))
	}
	if len(result.Unchanged) != 3 {
		t.Errorf("Unchanged: got %d, want 3", len(result.Unchanged))
	}
}

// TestComputeDiff_DisjointSets verifies that when manifest and local have
// completely different packages, manifest packages go to ToInstall and local
// packages go to ToRemove.
// Validates: Requirements 2.3, 2.4
func TestComputeDiff_DisjointSets(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Formulae: []manifest.PackageEntry{
			{Name: "git"},
			{Name: "go"},
		},
	}
	local := &LocalState{
		Formulae: []Package{
			{Name: "curl", Version: "8.0"},
			{Name: "wget", Version: "1.21"},
		},
	}

	result := ComputeDiff(m, local, "my-machine")

	if len(result.ToInstall) != 2 {
		t.Fatalf("ToInstall: got %d, want 2", len(result.ToInstall))
	}
	if _, ok := findEntry(result.ToInstall, "git"); !ok {
		t.Error("ToInstall missing 'git'")
	}
	if _, ok := findEntry(result.ToInstall, "go"); !ok {
		t.Error("ToInstall missing 'go'")
	}

	if len(result.ToRemove) != 2 {
		t.Fatalf("ToRemove: got %d, want 2", len(result.ToRemove))
	}
	if _, ok := findEntry(result.ToRemove, "curl"); !ok {
		t.Error("ToRemove missing 'curl'")
	}
	if _, ok := findEntry(result.ToRemove, "wget"); !ok {
		t.Error("ToRemove missing 'wget'")
	}

	if len(result.ToUpgrade) != 0 {
		t.Errorf("ToUpgrade: got %d, want 0", len(result.ToUpgrade))
	}
	if len(result.Unchanged) != 0 {
		t.Errorf("Unchanged: got %d, want 0", len(result.Unchanged))
	}
}

// TestComputeDiff_PartialOverlap verifies correct classification when some
// packages are shared and some are unique to each side.
// Validates: Requirements 2.2, 2.3, 2.4, 2.6
func TestComputeDiff_PartialOverlap(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Formulae: []manifest.PackageEntry{
			{Name: "git"},     // in both → Unchanged
			{Name: "go"},      // manifest only → ToInstall
			{Name: "ripgrep"}, // manifest only → ToInstall
		},
	}
	local := &LocalState{
		Formulae: []Package{
			{Name: "git", Version: "2.40"}, // in both → Unchanged
			{Name: "curl", Version: "8.0"}, // local only → ToRemove
		},
	}

	result := ComputeDiff(m, local, "my-machine")

	if len(result.ToInstall) != 2 {
		t.Errorf("ToInstall: got %d, want 2", len(result.ToInstall))
	}
	if _, ok := findEntry(result.ToInstall, "go"); !ok {
		t.Error("ToInstall missing 'go'")
	}
	if _, ok := findEntry(result.ToInstall, "ripgrep"); !ok {
		t.Error("ToInstall missing 'ripgrep'")
	}

	if len(result.ToRemove) != 1 {
		t.Errorf("ToRemove: got %d, want 1", len(result.ToRemove))
	}
	if _, ok := findEntry(result.ToRemove, "curl"); !ok {
		t.Error("ToRemove missing 'curl'")
	}

	if len(result.Unchanged) != 1 {
		t.Errorf("Unchanged: got %d, want 1", len(result.Unchanged))
	}
	if _, ok := findEntry(result.Unchanged, "git"); !ok {
		t.Error("Unchanged missing 'git'")
	}
}

// TestComputeDiff_VersionDifference verifies that a package present in both
// manifest and local with a different version in the manifest is classified
// as ToUpgrade.
// Validates: Requirements 2.5
func TestComputeDiff_VersionDifference(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Formulae: []manifest.PackageEntry{
			{Name: "go", Version: "1.24"},
		},
	}
	local := &LocalState{
		Formulae: []Package{
			{Name: "go", Version: "1.23"},
		},
	}

	result := ComputeDiff(m, local, "my-machine")

	if len(result.ToUpgrade) != 1 {
		t.Fatalf("ToUpgrade: got %d, want 1", len(result.ToUpgrade))
	}
	if result.ToUpgrade[0].Name != "go" {
		t.Errorf("ToUpgrade[0].Name: got %q, want %q", result.ToUpgrade[0].Name, "go")
	}
	if len(result.ToInstall) != 0 {
		t.Errorf("ToInstall: got %d, want 0", len(result.ToInstall))
	}
	if len(result.ToRemove) != 0 {
		t.Errorf("ToRemove: got %d, want 0", len(result.ToRemove))
	}
	if len(result.Unchanged) != 0 {
		t.Errorf("Unchanged: got %d, want 0", len(result.Unchanged))
	}
}

// TestComputeDiff_NoVersionInManifest verifies that when the manifest has no
// version specified for a package, it is classified as Unchanged regardless
// of the local version (no version = any version is fine).
// Validates: Requirements 2.6
func TestComputeDiff_NoVersionInManifest(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Formulae: []manifest.PackageEntry{
			{Name: "git"}, // no version
		},
	}
	local := &LocalState{
		Formulae: []Package{
			{Name: "git", Version: "2.40"},
		},
	}

	result := ComputeDiff(m, local, "my-machine")

	if len(result.Unchanged) != 1 {
		t.Fatalf("Unchanged: got %d, want 1", len(result.Unchanged))
	}
	if result.Unchanged[0].Name != "git" {
		t.Errorf("Unchanged[0].Name: got %q, want %q", result.Unchanged[0].Name, "git")
	}
	if len(result.ToUpgrade) != 0 {
		t.Errorf("ToUpgrade: got %d, want 0 (no version in manifest means any version is fine)", len(result.ToUpgrade))
	}
}

// ---------------------------------------------------------------------------
// Same-name formula/cask tests (validates the separate seen-maps fix)
// ---------------------------------------------------------------------------

// TestComputeDiff_SameNameFormulaCask_BothUnchanged verifies that a formula
// and cask with the same name are independently tracked as Unchanged when
// both exist in manifest and local.
func TestComputeDiff_SameNameFormulaCask_BothUnchanged(t *testing.T) {
	m := &manifest.Manifest{
		Version:  1,
		Formulae: []manifest.PackageEntry{{Name: "docker"}},
		Casks:    []manifest.PackageEntry{{Name: "docker"}},
	}
	local := &LocalState{
		Formulae: []Package{{Name: "docker", Version: "24.0"}},
		Casks:    []Package{{Name: "docker", Version: "4.25"}},
	}

	result := ComputeDiff(m, local, "my-machine")

	if len(result.Unchanged) != 2 {
		t.Errorf("Unchanged: got %d, want 2", len(result.Unchanged))
	}
	if len(result.ToRemove) != 0 {
		t.Errorf("ToRemove: got %v, want empty", entryNames(result.ToRemove))
	}
	if len(result.ToInstall) != 0 {
		t.Errorf("ToInstall: got %v, want empty", entryNames(result.ToInstall))
	}
}

// TestComputeDiff_SameNameFormulaCask_CaskLocalOnly verifies that a local-only
// cask is classified as ToRemove even when a formula with the same name exists
// in the manifest. This is the exact bug the separate seen-maps fix addresses:
// with a shared map, the cask would be shadowed by the formula and never
// appear in ToRemove.
func TestComputeDiff_SameNameFormulaCask_CaskLocalOnly(t *testing.T) {
	m := &manifest.Manifest{
		Version:  1,
		Formulae: []manifest.PackageEntry{{Name: "docker"}},
		// no cask named "docker" in manifest
	}
	local := &LocalState{
		Formulae: []Package{{Name: "docker", Version: "24.0"}},
		Casks:    []Package{{Name: "docker", Version: "4.25"}},
	}

	result := ComputeDiff(m, local, "my-machine")

	if len(result.Unchanged) != 1 {
		t.Errorf("Unchanged: got %d, want 1 (formula)", len(result.Unchanged))
	}
	if len(result.ToRemove) != 1 {
		t.Fatalf("ToRemove: got %d, want 1 (cask)", len(result.ToRemove))
	}
	if result.ToRemove[0].Name != "docker" {
		t.Errorf("ToRemove[0].Name: got %q, want %q", result.ToRemove[0].Name, "docker")
	}
}

// TestComputeDiff_SameNameFormulaCask_FormulaLocalOnly verifies the symmetric
// case: a local-only formula is classified as ToRemove even when a cask with
// the same name exists in the manifest.
func TestComputeDiff_SameNameFormulaCask_FormulaLocalOnly(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		// no formula named "docker" in manifest
		Casks: []manifest.PackageEntry{{Name: "docker"}},
	}
	local := &LocalState{
		Formulae: []Package{{Name: "docker", Version: "24.0"}},
		Casks:    []Package{{Name: "docker", Version: "4.25"}},
	}

	result := ComputeDiff(m, local, "my-machine")

	if len(result.Unchanged) != 1 {
		t.Errorf("Unchanged: got %d, want 1 (cask)", len(result.Unchanged))
	}
	if len(result.ToRemove) != 1 {
		t.Fatalf("ToRemove: got %d, want 1 (formula)", len(result.ToRemove))
	}
	if result.ToRemove[0].Name != "docker" {
		t.Errorf("ToRemove[0].Name: got %q, want %q", result.ToRemove[0].Name, "docker")
	}
}

// TestComputeDiff_SameNameFormulaCask_BothManifestOnlyOneLocal verifies that
// when both a formula and cask with the same name are in the manifest but only
// one type is installed locally, the missing type is ToInstall and the present
// type is Unchanged.
func TestComputeDiff_SameNameFormulaCask_BothManifestOnlyOneLocal(t *testing.T) {
	m := &manifest.Manifest{
		Version:  1,
		Formulae: []manifest.PackageEntry{{Name: "docker"}},
		Casks:    []manifest.PackageEntry{{Name: "docker"}},
	}
	local := &LocalState{
		Formulae: []Package{{Name: "docker", Version: "24.0"}},
		// no cask named "docker" locally
	}

	result := ComputeDiff(m, local, "my-machine")

	if len(result.Unchanged) != 1 {
		t.Errorf("Unchanged: got %d, want 1", len(result.Unchanged))
	}
	if len(result.ToInstall) != 1 {
		t.Fatalf("ToInstall: got %d, want 1", len(result.ToInstall))
	}
	if result.ToInstall[0].Name != "docker" {
		t.Errorf("ToInstall[0].Name: got %q, want %q", result.ToInstall[0].Name, "docker")
	}
	if len(result.ToRemove) != 0 {
		t.Errorf("ToRemove: got %v, want empty", entryNames(result.ToRemove))
	}
}

// TestComputeDiff_MachineFilterOnlyOn verifies that a package with only_on
// that doesn't match the current machineTag is excluded from ToInstall.
// Validates: Requirements 2.7, 3.1
func TestComputeDiff_MachineFilterOnlyOn(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Formulae: []manifest.PackageEntry{
			{Name: "docker", OnlyOn: []string{"work-laptop"}},
			{Name: "git"}, // no filter, always included
		},
	}
	local := &LocalState{}

	result := ComputeDiff(m, local, "home-desktop")

	// docker should be filtered out (only_on=work-laptop, machineTag=home-desktop)
	// git should be in ToInstall
	if len(result.ToInstall) != 1 {
		t.Fatalf("ToInstall: got %d, want 1", len(result.ToInstall))
	}
	if result.ToInstall[0].Name != "git" {
		t.Errorf("ToInstall[0].Name: got %q, want %q", result.ToInstall[0].Name, "git")
	}

	// docker should not appear anywhere in the result
	for _, e := range result.ToInstall {
		if e.Name == "docker" {
			t.Error("docker should be excluded by only_on filter but appeared in ToInstall")
		}
	}
	for _, e := range result.Unchanged {
		if e.Name == "docker" {
			t.Error("docker should be excluded by only_on filter but appeared in Unchanged")
		}
	}
}

// TestComputeDiff_MachineFilterExceptOn verifies that a package with except_on
// matching the current machineTag is excluded from ToInstall.
// Validates: Requirements 2.7, 3.2
func TestComputeDiff_MachineFilterExceptOn(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Casks: []manifest.PackageEntry{
			{Name: "slack", ExceptOn: []string{"home-desktop"}},
			{Name: "firefox"}, // no filter
		},
	}
	local := &LocalState{}

	result := ComputeDiff(m, local, "home-desktop")

	// slack should be filtered out (except_on includes home-desktop)
	// firefox should be in ToInstall
	if len(result.ToInstall) != 1 {
		t.Fatalf("ToInstall: got %d, want 1", len(result.ToInstall))
	}
	if result.ToInstall[0].Name != "firefox" {
		t.Errorf("ToInstall[0].Name: got %q, want %q", result.ToInstall[0].Name, "firefox")
	}
}

// TestComputeDiff_MachineFilterIncluded verifies that a package with only_on
// matching the current machineTag is included normally in the diff.
// Validates: Requirements 2.7, 3.1
func TestComputeDiff_MachineFilterIncluded(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Formulae: []manifest.PackageEntry{
			{Name: "docker", OnlyOn: []string{"work-laptop"}},
		},
	}
	local := &LocalState{}

	result := ComputeDiff(m, local, "work-laptop")

	// docker should be included because machineTag matches only_on
	if len(result.ToInstall) != 1 {
		t.Fatalf("ToInstall: got %d, want 1", len(result.ToInstall))
	}
	if result.ToInstall[0].Name != "docker" {
		t.Errorf("ToInstall[0].Name: got %q, want %q", result.ToInstall[0].Name, "docker")
	}
}

// ---------------------------------------------------------------------------
// FilterForMachine unit tests
// ---------------------------------------------------------------------------

// TestFilterForMachine_AllCases is a table-driven test covering all
// combinations of only_on, except_on, and no filter.
// Validates: Requirements 3.1, 3.2, 3.3
func TestFilterForMachine_AllCases(t *testing.T) {
	tests := []struct {
		name       string
		entries    []manifest.PackageEntry
		machineTag string
		wantNames  []string
	}{
		{
			name:       "only_on match — included",
			entries:    []manifest.PackageEntry{{Name: "docker", OnlyOn: []string{"work-laptop"}}},
			machineTag: "work-laptop",
			wantNames:  []string{"docker"},
		},
		{
			name:       "only_on mismatch — excluded",
			entries:    []manifest.PackageEntry{{Name: "docker", OnlyOn: []string{"work-laptop"}}},
			machineTag: "home-desktop",
			wantNames:  []string{},
		},
		{
			name:       "only_on multiple tags, one matches — included",
			entries:    []manifest.PackageEntry{{Name: "docker", OnlyOn: []string{"work-laptop", "ci-server"}}},
			machineTag: "ci-server",
			wantNames:  []string{"docker"},
		},
		{
			name:       "except_on match — excluded",
			entries:    []manifest.PackageEntry{{Name: "slack", ExceptOn: []string{"home-desktop"}}},
			machineTag: "home-desktop",
			wantNames:  []string{},
		},
		{
			name:       "except_on mismatch — included",
			entries:    []manifest.PackageEntry{{Name: "slack", ExceptOn: []string{"home-desktop"}}},
			machineTag: "work-laptop",
			wantNames:  []string{"slack"},
		},
		{
			name:       "except_on multiple tags, one matches — excluded",
			entries:    []manifest.PackageEntry{{Name: "slack", ExceptOn: []string{"home-desktop", "ci-server"}}},
			machineTag: "ci-server",
			wantNames:  []string{},
		},
		{
			name:       "no filter — always included",
			entries:    []manifest.PackageEntry{{Name: "git"}},
			machineTag: "any-machine",
			wantNames:  []string{"git"},
		},
		{
			name: "mixed entries — correct filtering",
			entries: []manifest.PackageEntry{
				{Name: "git"}, // no filter → included
				{Name: "docker", OnlyOn: []string{"work-laptop"}},     // only_on match → included
				{Name: "slack", ExceptOn: []string{"work-laptop"}},    // except_on match → excluded
				{Name: "vscode", OnlyOn: []string{"home-desktop"}},    // only_on mismatch → excluded
				{Name: "firefox", ExceptOn: []string{"home-desktop"}}, // except_on mismatch → included
			},
			machineTag: "work-laptop",
			wantNames:  []string{"git", "docker", "firefox"},
		},
		{
			name:       "empty entries — empty result",
			entries:    []manifest.PackageEntry{},
			machineTag: "my-machine",
			wantNames:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FilterForMachine(tt.entries, tt.machineTag)

			if len(result) != len(tt.wantNames) {
				t.Fatalf("got %d entries, want %d; got names: %v",
					len(result), len(tt.wantNames), entryNames(result))
			}

			gotNames := make(map[string]bool)
			for _, e := range result {
				gotNames[e.Name] = true
			}
			for _, want := range tt.wantNames {
				if !gotNames[want] {
					t.Errorf("expected %q in result, but not found; got: %v", want, entryNames(result))
				}
			}
		})
	}
}
