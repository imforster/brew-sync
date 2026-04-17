package manifest

import (
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"pgregory.net/rapid"
)

// normalizeManifest ensures nil and empty slices are treated consistently.
// TOML deserialization produces nil for absent/empty arrays, so we normalize
// empty slices to nil before comparison.
func normalizeManifest(m *Manifest) *Manifest {
	out := *m

	if len(out.Formulae) == 0 {
		out.Formulae = nil
	} else {
		normalized := make([]PackageEntry, len(out.Formulae))
		for i, e := range out.Formulae {
			normalized[i] = normalizePackageEntry(e)
		}
		out.Formulae = normalized
	}

	if len(out.Casks) == 0 {
		out.Casks = nil
	} else {
		normalized := make([]PackageEntry, len(out.Casks))
		for i, e := range out.Casks {
			normalized[i] = normalizePackageEntry(e)
		}
		out.Casks = normalized
	}

	if len(out.Taps) == 0 {
		out.Taps = nil
	}

	return &out
}

func normalizePackageEntry(e PackageEntry) PackageEntry {
	if len(e.OnlyOn) == 0 {
		e.OnlyOn = nil
	}
	if len(e.ExceptOn) == 0 {
		e.ExceptOn = nil
	}
	return e
}

// genName generates a valid non-empty package name (lowercase letters and hyphens).
func genName() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		// Generate a name of 1-20 lowercase letters, optionally with hyphens
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

// genTap generates a valid tap in owner/repo format.
func genTap() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		owner := genName().Draw(t, "owner")
		repo := genName().Draw(t, "repo")
		return owner + "/" + repo
	})
}

// genVersion generates an optional version string.
func genVersion() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		if rapid.Bool().Draw(t, "hasVersion") {
			major := rapid.IntRange(0, 99).Draw(t, "major")
			minor := rapid.IntRange(0, 99).Draw(t, "minor")
			return fmt.Sprintf("%d.%d", major, minor)
		}
		return ""
	})
}

// genMachineTags generates an optional list of machine tags.
func genMachineTags() *rapid.Generator[[]string] {
	return rapid.Custom(func(t *rapid.T) []string {
		count := rapid.IntRange(0, 3).Draw(t, "tagCount")
		if count == 0 {
			return nil
		}
		tags := make([]string, count)
		for i := range tags {
			tags[i] = genName().Draw(t, "tag")
		}
		return tags
	})
}

// genPackageEntry generates a valid PackageEntry.
// only_on and except_on are mutually exclusive per validation rules.
func genPackageEntry(name string) *rapid.Generator[PackageEntry] {
	return rapid.Custom(func(t *rapid.T) PackageEntry {
		entry := PackageEntry{
			Name:    name,
			Version: genVersion().Draw(t, "version"),
		}
		// Mutually exclusive: pick one of {none, only_on, except_on}
		filterChoice := rapid.IntRange(0, 2).Draw(t, "filterChoice")
		switch filterChoice {
		case 1:
			entry.OnlyOn = genMachineTags().Draw(t, "onlyOn")
		case 2:
			entry.ExceptOn = genMachineTags().Draw(t, "exceptOn")
		}
		return entry
	})
}

// genUniqueNames generates a slice of unique non-empty names.
func genUniqueNames(maxCount int) *rapid.Generator[[]string] {
	return rapid.Custom(func(t *rapid.T) []string {
		count := rapid.IntRange(0, maxCount).Draw(t, "count")
		seen := make(map[string]bool)
		names := make([]string, 0, count)
		for len(names) < count {
			name := genName().Draw(t, "name")
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
		return names
	})
}

// genManifest generates a valid Manifest struct.
func genManifest() *rapid.Generator[Manifest] {
	return rapid.Custom(func(t *rapid.T) Manifest {
		// Generate unique formulae names
		formulaeNames := genUniqueNames(5).Draw(t, "formulaeNames")
		formulae := make([]PackageEntry, len(formulaeNames))
		for i, name := range formulaeNames {
			formulae[i] = genPackageEntry(name).Draw(t, "formula")
		}

		// Generate unique cask names
		caskNames := genUniqueNames(5).Draw(t, "caskNames")
		casks := make([]PackageEntry, len(caskNames))
		for i, name := range caskNames {
			casks[i] = genPackageEntry(name).Draw(t, "cask")
		}

		// Generate unique taps
		tapCount := rapid.IntRange(0, 3).Draw(t, "tapCount")
		seen := make(map[string]bool)
		var taps []string
		for len(taps) < tapCount {
			tap := genTap().Draw(t, "tap")
			if !seen[tap] {
				seen[tap] = true
				taps = append(taps, tap)
			}
		}

		// Normalize: use nil instead of empty slices for consistency
		if len(formulae) == 0 {
			formulae = nil
		}
		if len(casks) == 0 {
			casks = nil
		}
		if len(taps) == 0 {
			taps = nil
		}

		return Manifest{
			Version: 1,
			Metadata: ManifestMetadata{
				UpdatedAt: rapid.StringMatching(`[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z`).Draw(t, "updatedAt"),
				UpdatedBy: genName().Draw(t, "updatedBy"),
				Machine:   genName().Draw(t, "machine"),
			},
			Formulae: formulae,
			Casks:    casks,
			Taps:     taps,
		}
	})
}

// TestPropertyManifestRoundTrip verifies Property 5: Load(Save(m)) == m for any valid manifest.
//
// **Validates: Requirements 8.1, 8.2**
//
// Requirement 8.1: THE Manifest_Manager SHALL serialize and deserialize manifests in TOML format
// preserving all fields including version, metadata, formulae, casks, and taps.
//
// Requirement 8.2: WHEN loading a manifest, THE Manifest_Manager SHALL produce an object that,
// when saved and loaded again, is equivalent to the original (round-trip preservation).
func TestPropertyManifestRoundTrip(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		original := genManifest().Draw(rt, "manifest")

		mgr := NewManifestManager()
		dir := t.TempDir()
		path := filepath.Join(dir, fmt.Sprintf("brew-sync-%d.toml", rapid.IntRange(0, 1000000).Draw(rt, "fileId")))

		// Save the manifest
		err := mgr.Save(path, &original)
		if err != nil {
			rt.Fatalf("Save failed: %v", err)
		}

		// Load it back
		loaded, err := mgr.Load(path)
		if err != nil {
			rt.Fatalf("Load failed: %v", err)
		}

		// Normalize both for comparison (nil vs empty slice consistency)
		normalizedOriginal := normalizeManifest(&original)
		normalizedLoaded := normalizeManifest(loaded)

		if !reflect.DeepEqual(normalizedOriginal, normalizedLoaded) {
			rt.Fatalf("round-trip mismatch:\noriginal:  %+v\nloaded:    %+v", normalizedOriginal, normalizedLoaded)
		}
	})
}

// ---------------------------------------------------------------------------
// Property 6: Validation completeness
// A manifest with duplicate names, empty names, both only_on and except_on
// set, or invalid tap format is rejected by Validate.
// **Validates: Requirements 8.3, 8.4, 8.5, 8.6, 8.7**
// ---------------------------------------------------------------------------

// validBaseManifest returns a minimal valid manifest that passes Validate.
func validBaseManifest() Manifest {
	return Manifest{
		Version: 1,
		Metadata: ManifestMetadata{
			UpdatedAt: "2025-01-01T00:00:00Z",
			UpdatedBy: "test",
			Machine:   "test-machine",
		},
		Formulae: []PackageEntry{{Name: "base-formula"}},
		Casks:    []PackageEntry{{Name: "base-cask"}},
		Taps:     []string{"owner/repo"},
	}
}

// TestPropertyValidateRejectsDuplicateFormulae generates a manifest with at
// least one duplicate formula name and asserts Validate returns an error.
//
// **Validates: Requirements 8.4**
func TestPropertyValidateRejectsDuplicateFormulae(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		m := validBaseManifest()

		// Generate a non-empty name that will be duplicated
		dupName := genName().Draw(rt, "dupName")

		// Build formulae list: at least two entries share the same name
		extraCount := rapid.IntRange(0, 4).Draw(rt, "extraCount")
		entries := []PackageEntry{
			{Name: dupName, Version: genVersion().Draw(rt, "v1")},
			{Name: dupName, Version: genVersion().Draw(rt, "v2")},
		}
		for i := 0; i < extraCount; i++ {
			// Use a name that won't collide with dupName
			entries = append(entries, PackageEntry{
				Name: fmt.Sprintf("unique-formula-%d", i),
			})
		}
		m.Formulae = entries

		mgr := NewManifestManager()
		err := mgr.Validate(&m)
		if err == nil {
			rt.Fatalf("expected Validate to reject manifest with duplicate formula %q, but got nil", dupName)
		}
	})
}

// TestPropertyValidateRejectsDuplicateCasks generates a manifest with at
// least one duplicate cask name and asserts Validate returns an error.
//
// **Validates: Requirements 8.4**
func TestPropertyValidateRejectsDuplicateCasks(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		m := validBaseManifest()

		dupName := genName().Draw(rt, "dupName")

		extraCount := rapid.IntRange(0, 4).Draw(rt, "extraCount")
		entries := []PackageEntry{
			{Name: dupName, Version: genVersion().Draw(rt, "v1")},
			{Name: dupName, Version: genVersion().Draw(rt, "v2")},
		}
		for i := 0; i < extraCount; i++ {
			entries = append(entries, PackageEntry{
				Name: fmt.Sprintf("unique-cask-%d", i),
			})
		}
		m.Casks = entries

		mgr := NewManifestManager()
		err := mgr.Validate(&m)
		if err == nil {
			rt.Fatalf("expected Validate to reject manifest with duplicate cask %q, but got nil", dupName)
		}
	})
}

// TestPropertyValidateRejectsEmptyNames generates a manifest with at least
// one empty name in formulae or casks and asserts Validate returns an error.
//
// **Validates: Requirements 8.5**
func TestPropertyValidateRejectsEmptyNames(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		m := validBaseManifest()

		// Decide whether to inject the empty name in formulae or casks (or both)
		inFormulae := rapid.Bool().Draw(rt, "inFormulae")

		if inFormulae {
			// Inject an empty-name entry into formulae
			extraCount := rapid.IntRange(0, 3).Draw(rt, "extraFormulae")
			entries := []PackageEntry{{Name: ""}}
			for i := 0; i < extraCount; i++ {
				entries = append(entries, PackageEntry{
					Name: fmt.Sprintf("valid-formula-%d", i),
				})
			}
			m.Formulae = entries
		} else {
			// Inject an empty-name entry into casks
			extraCount := rapid.IntRange(0, 3).Draw(rt, "extraCasks")
			entries := []PackageEntry{{Name: ""}}
			for i := 0; i < extraCount; i++ {
				entries = append(entries, PackageEntry{
					Name: fmt.Sprintf("valid-cask-%d", i),
				})
			}
			m.Casks = entries
		}

		mgr := NewManifestManager()
		err := mgr.Validate(&m)
		if err == nil {
			rt.Fatal("expected Validate to reject manifest with empty name, but got nil")
		}
	})
}

// TestPropertyValidateRejectsBothOnlyOnAndExceptOn generates a manifest where
// at least one entry has both only_on and except_on set and asserts Validate
// returns an error.
//
// **Validates: Requirements 8.6**
func TestPropertyValidateRejectsBothOnlyOnAndExceptOn(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		m := validBaseManifest()

		// Generate an entry with both only_on and except_on set (non-empty)
		name := genName().Draw(rt, "name")
		onlyOnCount := rapid.IntRange(1, 3).Draw(rt, "onlyOnCount")
		exceptOnCount := rapid.IntRange(1, 3).Draw(rt, "exceptOnCount")

		onlyOn := make([]string, onlyOnCount)
		for i := range onlyOn {
			onlyOn[i] = genName().Draw(rt, "onlyOnTag")
		}
		exceptOn := make([]string, exceptOnCount)
		for i := range exceptOn {
			exceptOn[i] = genName().Draw(rt, "exceptOnTag")
		}

		badEntry := PackageEntry{
			Name:     name,
			OnlyOn:   onlyOn,
			ExceptOn: exceptOn,
		}

		// Inject into formulae or casks randomly
		inFormulae := rapid.Bool().Draw(rt, "inFormulae")
		if inFormulae {
			m.Formulae = append(m.Formulae, badEntry)
		} else {
			m.Casks = append(m.Casks, badEntry)
		}

		mgr := NewManifestManager()
		err := mgr.Validate(&m)
		if err == nil {
			rt.Fatalf("expected Validate to reject manifest with both only_on and except_on on %q, but got nil", name)
		}
	})
}

// TestPropertyValidateRejectsInvalidTapFormat generates a manifest with at
// least one tap that doesn't match the owner/repo format and asserts Validate
// returns an error.
//
// **Validates: Requirements 8.7**
func TestPropertyValidateRejectsInvalidTapFormat(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		m := validBaseManifest()

		// Generate an invalid tap: pick from several invalid patterns
		invalidChoice := rapid.IntRange(0, 4).Draw(rt, "invalidChoice")
		var invalidTap string
		switch invalidChoice {
		case 0:
			// No slash at all
			invalidTap = genName().Draw(rt, "noSlash")
		case 1:
			// Empty owner (starts with /)
			invalidTap = "/" + genName().Draw(rt, "repo")
		case 2:
			// Empty repo (ends with /)
			invalidTap = genName().Draw(rt, "owner") + "/"
		case 3:
			// Multiple slashes
			invalidTap = genName().Draw(rt, "p1") + "/" + genName().Draw(rt, "p2") + "/" + genName().Draw(rt, "p3")
		case 4:
			// Empty string
			invalidTap = ""
		}

		// Mix with valid taps
		validCount := rapid.IntRange(0, 2).Draw(rt, "validTapCount")
		taps := []string{invalidTap}
		for i := 0; i < validCount; i++ {
			taps = append(taps, genTap().Draw(rt, "validTap"))
		}
		m.Taps = taps

		mgr := NewManifestManager()
		err := mgr.Validate(&m)
		if err == nil {
			rt.Fatalf("expected Validate to reject manifest with invalid tap %q, but got nil", invalidTap)
		}
	})
}

// TestPropertyValidateRejectsUnsupportedVersion generates a manifest with
// version != 1 and asserts Validate returns an error.
//
// **Validates: Requirements 8.3**
func TestPropertyValidateRejectsUnsupportedVersion(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		m := validBaseManifest()

		// Generate a version that is not 1
		version := rapid.IntRange(-100, 100).Filter(func(v int) bool {
			return v != 1
		}).Draw(rt, "badVersion")
		m.Version = version

		mgr := NewManifestManager()
		err := mgr.Validate(&m)
		if err == nil {
			rt.Fatalf("expected Validate to reject manifest with version %d, but got nil", version)
		}
	})
}
