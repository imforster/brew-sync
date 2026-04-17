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

// diffTestData holds generated manifest and local state with names that are
// unique across all four lists (manifest formulae, manifest casks, local
// formulae, local casks). This matches the engine's assumption that package
// names are unique across formulae and casks.
type diffTestData struct {
	Manifest *manifest.Manifest
	Local    *LocalState
}

// genDiffTestData generates a manifest and local state where all package names
// are unique across manifest formulae, manifest casks, local formulae, and
// local casks. This avoids name collisions between types, which the engine's
// single `seen` map assumes.
func genDiffTestData() *rapid.Generator[diffTestData] {
	return rapid.Custom(func(t *rapid.T) diffTestData {
		// Generate a pool of unique names, then partition them across the four lists.
		totalNames := rapid.IntRange(0, 20).Draw(t, "totalNames")
		seen := make(map[string]bool)
		allNames := make([]string, 0, totalNames)
		for len(allNames) < totalNames {
			name := genDiffPkgName().Draw(t, "name")
			if !seen[name] {
				seen[name] = true
				allNames = append(allNames, name)
			}
		}

		// Assign each name to one or more of the four buckets:
		// 0 = manifest formulae, 1 = manifest casks,
		// 2 = local formulae, 3 = local casks.
		// Each name goes to exactly one formula bucket and/or one cask bucket
		// to keep names unique within each namespace.
		// But since the engine uses a single seen map, we keep names globally unique.
		var manifestFormulae []manifest.PackageEntry
		var manifestCasks []manifest.PackageEntry
		var localFormulae []Package
		var localCasks []Package

		for _, name := range allNames {
			// Each name can appear in manifest (as formula or cask) and/or
			// local (as formula or cask). We assign to exactly one type on
			// each side to keep things clean.
			inManifest := rapid.Bool().Draw(t, "inManifest")
			inLocal := rapid.Bool().Draw(t, "inLocal")

			// Ensure at least one side has the name
			if !inManifest && !inLocal {
				if rapid.Bool().Draw(t, "defaultSide") {
					inManifest = true
				} else {
					inLocal = true
				}
			}

			// Decide type: formula or cask (same type on both sides for simplicity)
			isFormula := rapid.Bool().Draw(t, "isFormula")

			if inManifest {
				version := genDiffVersion().Draw(t, "manifestVersion")
				entry := manifest.PackageEntry{Name: name, Version: version}
				if isFormula {
					manifestFormulae = append(manifestFormulae, entry)
				} else {
					manifestCasks = append(manifestCasks, entry)
				}
			}

			if inLocal {
				version := genDiffVersion().Draw(t, "localVersion")
				pkg := Package{Name: name, Version: version}
				if isFormula {
					localFormulae = append(localFormulae, pkg)
				} else {
					localCasks = append(localCasks, pkg)
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
// Every package in manifest ∪ local appears in exactly one of
// {ToInstall, ToRemove, ToUpgrade, Unchanged}.
//
// **Validates: Requirements 2.2, 2.3, 2.4, 2.5, 2.6**
// ---------------------------------------------------------------------------

// TestPropertyDiffCompleteness generates random manifests and local states,
// computes the diff, and verifies that every package in the union of
// (manifest entries + local packages) appears in exactly one of the four
// result sets. No package should be missing or appear in multiple sets.
func TestPropertyDiffCompleteness(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		data := genDiffTestData().Draw(rt, "testData")
		machineTag := "test-machine"

		result := ComputeDiff(data.Manifest, data.Local, machineTag)

		// Build the universe: all package names from manifest ∪ local.
		// Since we generate without machine filters, all manifest entries
		// pass through FilterForMachine.
		universe := make(map[string]bool)
		for _, e := range data.Manifest.Formulae {
			universe[e.Name] = true
		}
		for _, e := range data.Manifest.Casks {
			universe[e.Name] = true
		}
		for _, p := range data.Local.Formulae {
			universe[p.Name] = true
		}
		for _, p := range data.Local.Casks {
			universe[p.Name] = true
		}

		// Count how many times each name appears across the four result sets.
		counts := make(map[string]int)
		for _, e := range result.ToInstall {
			counts[e.Name]++
		}
		for _, e := range result.ToRemove {
			counts[e.Name]++
		}
		for _, e := range result.ToUpgrade {
			counts[e.Name]++
		}
		for _, e := range result.Unchanged {
			counts[e.Name]++
		}

		// Every package in the universe must appear exactly once.
		for name := range universe {
			c := counts[name]
			if c != 1 {
				rt.Fatalf("package %q appears %d times in diff result sets (expected exactly 1)", name, c)
			}
		}

		// No package outside the universe should appear in any result set.
		for name, c := range counts {
			if !universe[name] {
				rt.Fatalf("package %q appears %d times in diff result but is not in manifest ∪ local", name, c)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Property 2: Diff soundness
// ToInstall contains only manifest-only packages;
// ToRemove contains only local-only packages.
//
// **Validates: Requirements 2.2, 2.3, 2.4, 2.5, 2.6**
// ---------------------------------------------------------------------------

// TestPropertyDiffSoundness generates random manifests and local states,
// computes the diff, and verifies:
//   - Every package in ToInstall is in the manifest but NOT in local
//   - Every package in ToRemove is in local but NOT in the (filtered) manifest
func TestPropertyDiffSoundness(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		data := genDiffTestData().Draw(rt, "testData")
		machineTag := "test-machine"

		result := ComputeDiff(data.Manifest, data.Local, machineTag)

		// Build manifest name set (no filters, so all entries are included).
		manifestNames := make(map[string]bool)
		for _, e := range data.Manifest.Formulae {
			manifestNames[e.Name] = true
		}
		for _, e := range data.Manifest.Casks {
			manifestNames[e.Name] = true
		}

		// Build local name set.
		localNames := make(map[string]bool)
		for _, p := range data.Local.Formulae {
			localNames[p.Name] = true
		}
		for _, p := range data.Local.Casks {
			localNames[p.Name] = true
		}

		// ToInstall: every package must be in manifest but NOT in local.
		for _, e := range result.ToInstall {
			if !manifestNames[e.Name] {
				rt.Fatalf("ToInstall contains %q which is NOT in the manifest", e.Name)
			}
			if localNames[e.Name] {
				rt.Fatalf("ToInstall contains %q which IS in local (should not be ToInstall)", e.Name)
			}
		}

		// ToRemove: every package must be in local but NOT in manifest.
		for _, e := range result.ToRemove {
			if !localNames[e.Name] {
				rt.Fatalf("ToRemove contains %q which is NOT in local", e.Name)
			}
			if manifestNames[e.Name] {
				rt.Fatalf("ToRemove contains %q which IS in the manifest (should not be ToRemove)", e.Name)
			}
		}
	})
}
