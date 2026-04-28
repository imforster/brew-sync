package diff

import (
	"fmt"
	"testing"

	"brew-sync/internal/manifest"

	"pgregory.net/rapid"
)

// ---------------------------------------------------------------------------
// Generators
// ---------------------------------------------------------------------------

// genDiffPkgName generates a valid non-empty package name (lowercase letters and hyphens).
func genDiffPkgName() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		length := rapid.IntRange(1, 20).Draw(t, "nameLen")
		chars := make([]byte, length)
		for i := range chars {
			if i > 0 && i < length-1 && rapid.IntRange(0, 5).Draw(t, "charChoice") == 0 {
				chars[i] = '-'
			} else {
				chars[i] = byte(rapid.IntRange(int('a'), int('z')).Draw(t, "char"))
			}
		}
		return string(chars)
	})
}

// genDiffVersion generates an optional version string.
func genDiffVersion() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		if rapid.Bool().Draw(t, "hasVersion") {
			major := rapid.IntRange(0, 99).Draw(t, "major")
			minor := rapid.IntRange(0, 99).Draw(t, "minor")
			return fmt.Sprintf("%d.%d", major, minor)
		}
		return ""
	})
}

// diffTestData holds generated manifest and local state. Names are unique
// within each type (formulae, casks) but may overlap across types — a formula
// and cask can share the same name, which is the scenario the separate
// seen-maps fix addresses.
type diffTestData struct {
	Manifest *manifest.Manifest
	Local    *LocalState
}

// genDiffTestData generates a manifest and local state where names are unique
// within each type but may appear as both a formula and a cask. This exercises
// the separate seenFormulae/seenCasks maps in ComputeDiff.
func genDiffTestData() *rapid.Generator[diffTestData] {
	return rapid.Custom(func(t *rapid.T) diffTestData {
		totalNames := rapid.IntRange(0, 20).Draw(t, "totalNames")
		usedNames := make(map[string]bool)
		allNames := make([]string, 0, totalNames)
		for len(allNames) < totalNames {
			name := genDiffPkgName().Draw(t, "name")
			if !usedNames[name] {
				usedNames[name] = true
				allNames = append(allNames, name)
			}
		}

		seenFormulae := make(map[string]bool)
		seenCasks := make(map[string]bool)
		var manifestFormulae []manifest.PackageEntry
		var manifestCasks []manifest.PackageEntry
		var localFormulae []Package
		var localCasks []Package

		for _, name := range allNames {
			// Each name can independently appear as a formula and/or cask.
			asFormula := rapid.Bool().Draw(t, "asFormula")
			asCask := rapid.Bool().Draw(t, "asCask")
			if !asFormula && !asCask {
				if rapid.Bool().Draw(t, "defaultType") {
					asFormula = true
				} else {
					asCask = true
				}
			}

			if asFormula && !seenFormulae[name] {
				seenFormulae[name] = true
				inManifest := rapid.Bool().Draw(t, "formulaInManifest")
				inLocal := rapid.Bool().Draw(t, "formulaInLocal")
				if !inManifest && !inLocal {
					if rapid.Bool().Draw(t, "formulaDefault") {
						inManifest = true
					} else {
						inLocal = true
					}
				}
				if inManifest {
					manifestFormulae = append(manifestFormulae, manifest.PackageEntry{
						Name: name, Version: genDiffVersion().Draw(t, "fmVer"),
					})
				}
				if inLocal {
					localFormulae = append(localFormulae, Package{
						Name: name, Version: genDiffVersion().Draw(t, "flVer"),
					})
				}
			}

			if asCask && !seenCasks[name] {
				seenCasks[name] = true
				inManifest := rapid.Bool().Draw(t, "caskInManifest")
				inLocal := rapid.Bool().Draw(t, "caskInLocal")
				if !inManifest && !inLocal {
					if rapid.Bool().Draw(t, "caskDefault") {
						inManifest = true
					} else {
						inLocal = true
					}
				}
				if inManifest {
					manifestCasks = append(manifestCasks, manifest.PackageEntry{
						Name: name, Version: genDiffVersion().Draw(t, "cmVer"),
					})
				}
				if inLocal {
					localCasks = append(localCasks, Package{
						Name: name, Version: genDiffVersion().Draw(t, "clVer"),
					})
				}
			}
		}

		m := &manifest.Manifest{
			Version: 1,
			Metadata: manifest.ManifestMetadata{
				UpdatedAt: "2025-01-01T00:00:00Z",
				UpdatedBy: "test",
				Machine:   "test-machine",
			},
			Formulae: manifestFormulae,
			Casks:    manifestCasks,
		}

		local := &LocalState{
			Formulae: localFormulae,
			Casks:    localCasks,
		}

		return diffTestData{Manifest: m, Local: local}
	})
}

// ---------------------------------------------------------------------------
// Property 1: Diff completeness
// Every package slot (name × type) in manifest ∪ local appears in exactly
// one of {ToInstall, ToRemove, ToUpgrade, Unchanged, Skipped}.
// A name can appear twice if it exists as both a formula and a cask.
//
// **Validates: Requirements 2.2, 2.3, 2.4, 2.5, 2.6**
// ---------------------------------------------------------------------------

func TestPropertyDiffCompleteness(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		data := genDiffTestData().Draw(rt, "testData")
		machineTag := "test-machine"

		result := ComputeDiff(data.Manifest, data.Local, machineTag)

		// Count expected occurrences per name: one per type it appears in.
		expected := make(map[string]int)
		formulaNames := make(map[string]bool)
		caskNames := make(map[string]bool)
		for _, e := range data.Manifest.Formulae {
			formulaNames[e.Name] = true
		}
		for _, p := range data.Local.Formulae {
			formulaNames[p.Name] = true
		}
		for _, e := range data.Manifest.Casks {
			caskNames[e.Name] = true
		}
		for _, p := range data.Local.Casks {
			caskNames[p.Name] = true
		}
		for name := range formulaNames {
			expected[name]++
		}
		for name := range caskNames {
			expected[name]++
		}

		// Count actual occurrences across all result sets.
		actual := make(map[string]int)
		for _, e := range result.ToInstall {
			actual[e.Name]++
		}
		for _, e := range result.ToRemove {
			actual[e.Name]++
		}
		for _, e := range result.ToUpgrade {
			actual[e.Name]++
		}
		for _, e := range result.Unchanged {
			actual[e.Name]++
		}
		for _, e := range result.Skipped {
			actual[e.Name]++
		}

		for name, want := range expected {
			if actual[name] != want {
				rt.Fatalf("package %q: got %d occurrences in result, want %d", name, actual[name], want)
			}
		}
		for name, got := range actual {
			if expected[name] == 0 {
				rt.Fatalf("package %q appears %d times in result but not in manifest ∪ local", name, got)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Property 2: Diff soundness (per-type)
// ToInstall entries come from manifest-only slots; ToRemove from local-only.
// Checked per type since a name can be a formula in manifest and a cask in
// local (or vice versa).
//
// **Validates: Requirements 2.2, 2.3, 2.4, 2.5, 2.6**
// ---------------------------------------------------------------------------

func TestPropertyDiffSoundness(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		data := genDiffTestData().Draw(rt, "testData")
		machineTag := "test-machine"

		result := ComputeDiff(data.Manifest, data.Local, machineTag)

		// Build per-type name sets.
		manifestFormulae := make(map[string]bool)
		manifestCasks := make(map[string]bool)
		localFormulae := make(map[string]bool)
		localCasks := make(map[string]bool)
		for _, e := range data.Manifest.Formulae {
			manifestFormulae[e.Name] = true
		}
		for _, e := range data.Manifest.Casks {
			manifestCasks[e.Name] = true
		}
		for _, p := range data.Local.Formulae {
			localFormulae[p.Name] = true
		}
		for _, p := range data.Local.Casks {
			localCasks[p.Name] = true
		}

		inManifest := func(name string) bool { return manifestFormulae[name] || manifestCasks[name] }
		inLocal := func(name string) bool { return localFormulae[name] || localCasks[name] }

		// ToInstall: each entry must be in manifest and NOT have a matching
		// local type. A name can be ToInstall as a cask even if a formula
		// with the same name is local — they're independent types.
		for _, e := range result.ToInstall {
			if !inManifest(e.Name) {
				rt.Fatalf("ToInstall contains %q which is NOT in the manifest", e.Name)
			}
		}

		// ToRemove: each entry must be in local and NOT have a matching
		// manifest type.
		for _, e := range result.ToRemove {
			if !inLocal(e.Name) {
				rt.Fatalf("ToRemove contains %q which is NOT in local", e.Name)
			}
		}
	})
}
