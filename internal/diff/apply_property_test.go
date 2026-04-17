package diff

import (
	"fmt"
	"testing"

	"brew-sync/internal/manifest"

	"pgregory.net/rapid"
)

// ---------------------------------------------------------------------------
// Mock Runner
// ---------------------------------------------------------------------------

// testMockRunner implements the Runner interface and tracks call counts
// for Install, Uninstall, and Upgrade. Used to verify dry-run safety.
type testMockRunner struct {
	InstallCount   int
	UninstallCount int
	UpgradeCount   int
}

func (m *testMockRunner) Install(pkg Package) error {
	m.InstallCount++
	return nil
}

func (m *testMockRunner) Uninstall(pkg Package) error {
	m.UninstallCount++
	return nil
}

func (m *testMockRunner) Upgrade(pkg Package) error {
	m.UpgradeCount++
	return nil
}

// ---------------------------------------------------------------------------
// Generators
// ---------------------------------------------------------------------------

// genApplyPkgName generates a valid non-empty package name.
func genApplyPkgName() *rapid.Generator[string] {
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

// genApplyVersion generates an optional version string.
func genApplyVersion() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		if rapid.Bool().Draw(t, "hasVersion") {
			major := rapid.IntRange(0, 99).Draw(t, "major")
			minor := rapid.IntRange(0, 99).Draw(t, "minor")
			return fmt.Sprintf("%d.%d", major, minor)
		}
		return ""
	})
}

// genPackageEntrySlice generates a slice of manifest.PackageEntry with unique names.
func genPackageEntrySlice() *rapid.Generator[[]manifest.PackageEntry] {
	return rapid.Custom(func(t *rapid.T) []manifest.PackageEntry {
		count := rapid.IntRange(0, 10).Draw(t, "count")
		seen := make(map[string]bool)
		entries := make([]manifest.PackageEntry, 0, count)
		for len(entries) < count {
			name := genApplyPkgName().Draw(t, "name")
			if seen[name] {
				continue
			}
			seen[name] = true
			version := genApplyVersion().Draw(t, "version")
			entries = append(entries, manifest.PackageEntry{
				Name:    name,
				Version: version,
			})
		}
		return entries
	})
}

// genDiffResult generates a random DiffResult with random packages in
// ToInstall, ToRemove, and ToUpgrade.
func genDiffResult() *rapid.Generator[*DiffResult] {
	return rapid.Custom(func(t *rapid.T) *DiffResult {
		return &DiffResult{
			ToInstall: genPackageEntrySlice().Draw(t, "toInstall"),
			ToRemove:  genPackageEntrySlice().Draw(t, "toRemove"),
			ToUpgrade: genPackageEntrySlice().Draw(t, "toUpgrade"),
			Unchanged: genPackageEntrySlice().Draw(t, "unchanged"),
		}
	})
}

// ---------------------------------------------------------------------------
// Idempotency test data generator
// ---------------------------------------------------------------------------

// idempotencyTestData holds generated manifest and local state with names that
// are unique across all four lists (manifest formulae, manifest casks, local
// formulae, local casks). No machine filters are set.
type idempotencyTestData struct {
	Manifest *manifest.Manifest
	Local    *LocalState
}

// genIdempotencyTestData generates a manifest and local state where all package
// names are unique across manifest formulae, manifest casks, local formulae,
// and local casks. No OnlyOn/ExceptOn filters are set.
func genIdempotencyTestData() *rapid.Generator[idempotencyTestData] {
	return rapid.Custom(func(t *rapid.T) idempotencyTestData {
		totalNames := rapid.IntRange(0, 20).Draw(t, "totalNames")
		seen := make(map[string]bool)
		allNames := make([]string, 0, totalNames)
		for len(allNames) < totalNames {
			name := genApplyPkgName().Draw(t, "name")
			if !seen[name] {
				seen[name] = true
				allNames = append(allNames, name)
			}
		}

		var manifestFormulae []manifest.PackageEntry
		var manifestCasks []manifest.PackageEntry
		var localFormulae []Package
		var localCasks []Package

		for _, name := range allNames {
			inManifest := rapid.Bool().Draw(t, "inManifest")
			inLocal := rapid.Bool().Draw(t, "inLocal")

			if !inManifest && !inLocal {
				if rapid.Bool().Draw(t, "defaultSide") {
					inManifest = true
				} else {
					inLocal = true
				}
			}

			isFormula := rapid.Bool().Draw(t, "isFormula")

			if inManifest {
				version := genApplyVersion().Draw(t, "manifestVersion")
				entry := manifest.PackageEntry{Name: name, Version: version}
				if isFormula {
					manifestFormulae = append(manifestFormulae, entry)
				} else {
					manifestCasks = append(manifestCasks, entry)
				}
			}

			if inLocal {
				version := genApplyVersion().Draw(t, "localVersion")
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

		return idempotencyTestData{Manifest: m, Local: local}
	})
}

// ---------------------------------------------------------------------------
// Property 4: Idempotency
// Applying a diff and recomputing the diff yields an empty diff
// (no installs, no removes, no upgrades).
//
// **Validates: Requirements 4.1, 2.2**
// ---------------------------------------------------------------------------

func TestPropertyIdempotency(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		data := genIdempotencyTestData().Draw(rt, "testData")
		machineTag := "test-machine"

		// Step 1: Compute the initial diff.
		diff1 := ComputeDiff(data.Manifest, data.Local, machineTag)

		// Step 2: Simulate applying the diff by updating the local state.
		// Build mutable maps of local packages for easy manipulation.
		localFormulaeMap := make(map[string]string, len(data.Local.Formulae))
		for _, pkg := range data.Local.Formulae {
			localFormulaeMap[pkg.Name] = pkg.Version
		}
		localCasksMap := make(map[string]string, len(data.Local.Casks))
		for _, pkg := range data.Local.Casks {
			localCasksMap[pkg.Name] = pkg.Version
		}

		// Build a set of manifest formulae/cask names to know which type
		// each ToInstall/ToUpgrade package belongs to.
		manifestFormulaeNames := make(map[string]bool)
		for _, e := range data.Manifest.Formulae {
			manifestFormulaeNames[e.Name] = true
		}
		manifestCaskNames := make(map[string]bool)
		for _, e := range data.Manifest.Casks {
			manifestCaskNames[e.Name] = true
		}

		// Add ToInstall packages to local state.
		for _, entry := range diff1.ToInstall {
			version := entry.Version
			if manifestFormulaeNames[entry.Name] {
				localFormulaeMap[entry.Name] = version
			} else {
				localCasksMap[entry.Name] = version
			}
		}

		// Remove ToRemove packages from local state.
		for _, entry := range diff1.ToRemove {
			delete(localFormulaeMap, entry.Name)
			delete(localCasksMap, entry.Name)
		}

		// Update ToUpgrade packages in local state with the manifest version.
		for _, entry := range diff1.ToUpgrade {
			if manifestFormulaeNames[entry.Name] {
				localFormulaeMap[entry.Name] = entry.Version
			} else {
				localCasksMap[entry.Name] = entry.Version
			}
		}

		// Rebuild the local state from the maps.
		updatedLocal := &LocalState{}
		for name, version := range localFormulaeMap {
			updatedLocal.Formulae = append(updatedLocal.Formulae, Package{Name: name, Version: version})
		}
		for name, version := range localCasksMap {
			updatedLocal.Casks = append(updatedLocal.Casks, Package{Name: name, Version: version})
		}

		// Step 3: Recompute the diff with the updated local state.
		diff2 := ComputeDiff(data.Manifest, updatedLocal, machineTag)

		// Step 4: Assert the second diff is empty.
		if len(diff2.ToInstall) != 0 {
			rt.Fatalf("expected empty ToInstall after convergence, got %d: %v",
				len(diff2.ToInstall), diff2.ToInstall)
		}
		if len(diff2.ToRemove) != 0 {
			rt.Fatalf("expected empty ToRemove after convergence, got %d: %v",
				len(diff2.ToRemove), diff2.ToRemove)
		}
		if len(diff2.ToUpgrade) != 0 {
			rt.Fatalf("expected empty ToUpgrade after convergence, got %d: %v",
				len(diff2.ToUpgrade), diff2.ToUpgrade)
		}
	})
}

// ---------------------------------------------------------------------------
// Property 7: Dry-run safety
// When dryRun = true, ApplyDiff produces a report but makes zero calls to
// runner.Install, runner.Uninstall, or runner.Upgrade.
//
// **Validates: Requirements 5.1, 5.2**
// ---------------------------------------------------------------------------

func TestPropertyDryRunSafety(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		diff := genDiffResult().Draw(rt, "diff")
		runner := &testMockRunner{}

		report, err := ApplyDiff(diff, runner, true)

		// ApplyDiff with dryRun=true should not return an error.
		if err != nil {
			rt.Fatalf("ApplyDiff with dryRun=true returned error: %v", err)
		}

		// The runner must have received zero calls.
		if runner.InstallCount != 0 {
			rt.Fatalf("expected 0 Install calls in dry-run, got %d", runner.InstallCount)
		}
		if runner.UninstallCount != 0 {
			rt.Fatalf("expected 0 Uninstall calls in dry-run, got %d", runner.UninstallCount)
		}
		if runner.UpgradeCount != 0 {
			rt.Fatalf("expected 0 Upgrade calls in dry-run, got %d", runner.UpgradeCount)
		}

		// The report must have Planned=true.
		if !report.Planned {
			rt.Fatalf("expected report.Planned=true in dry-run, got false")
		}

		// The report counts must match the diff sizes.
		if report.InstallCount != len(diff.ToInstall) {
			rt.Fatalf("expected InstallCount=%d, got %d", len(diff.ToInstall), report.InstallCount)
		}
		if report.RemoveCount != len(diff.ToRemove) {
			rt.Fatalf("expected RemoveCount=%d, got %d", len(diff.ToRemove), report.RemoveCount)
		}
		if report.UpgradeCount != len(diff.ToUpgrade) {
			rt.Fatalf("expected UpgradeCount=%d, got %d", len(diff.ToUpgrade), report.UpgradeCount)
		}
	})
}
