package diff

import (
	"testing"

	"brew-sync/internal/manifest"

	"pgregory.net/rapid"
)

// ---------------------------------------------------------------------------
// Generators
// ---------------------------------------------------------------------------

// genMachineTag generates a machine tag string (lowercase letters and hyphens).
func genMachineTag() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		length := rapid.IntRange(1, 15).Draw(t, "tagLen")
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

// genPkgName generates a valid non-empty package name.
func genPkgName() *rapid.Generator[string] {
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

// genMachineTagDistinctFrom generates a machine tag guaranteed to differ from the given tag.
func genMachineTagDistinctFrom(excluded string) *rapid.Generator[string] {
	return genMachineTag().Filter(func(s string) bool {
		return s != excluded
	})
}

// genPackageEntryWithOnlyOn generates a PackageEntry whose OnlyOn contains
// the provided tags (and nothing else).
func genPackageEntryWithOnlyOn(tags []string) *rapid.Generator[manifest.PackageEntry] {
	return rapid.Custom(func(t *rapid.T) manifest.PackageEntry {
		return manifest.PackageEntry{
			Name:   genPkgName().Draw(t, "name"),
			OnlyOn: tags,
		}
	})
}

// genPackageEntryWithExceptOn generates a PackageEntry whose ExceptOn contains
// the provided tags.
func genPackageEntryWithExceptOn(tags []string) *rapid.Generator[manifest.PackageEntry] {
	return rapid.Custom(func(t *rapid.T) manifest.PackageEntry {
		return manifest.PackageEntry{
			Name:     genPkgName().Draw(t, "name"),
			ExceptOn: tags,
		}
	})
}

// genPackageEntryNoFilter generates a PackageEntry with neither OnlyOn nor ExceptOn.
func genPackageEntryNoFilter() *rapid.Generator[manifest.PackageEntry] {
	return rapid.Custom(func(t *rapid.T) manifest.PackageEntry {
		return manifest.PackageEntry{
			Name: genPkgName().Draw(t, "name"),
		}
	})
}

// genMixedEntries generates a random list of PackageEntry values with a mix
// of OnlyOn, ExceptOn, and no-filter entries.
func genMixedEntries() *rapid.Generator[[]manifest.PackageEntry] {
	return rapid.Custom(func(t *rapid.T) []manifest.PackageEntry {
		count := rapid.IntRange(0, 15).Draw(t, "entryCount")
		entries := make([]manifest.PackageEntry, count)
		for i := range entries {
			filterChoice := rapid.IntRange(0, 2).Draw(t, "filterChoice")
			name := genPkgName().Draw(t, "name")
			switch filterChoice {
			case 0:
				// No filter
				entries[i] = manifest.PackageEntry{Name: name}
			case 1:
				// OnlyOn with 1-3 tags
				tagCount := rapid.IntRange(1, 3).Draw(t, "onlyOnCount")
				tags := make([]string, tagCount)
				for j := range tags {
					tags[j] = genMachineTag().Draw(t, "onlyOnTag")
				}
				entries[i] = manifest.PackageEntry{Name: name, OnlyOn: tags}
			case 2:
				// ExceptOn with 1-3 tags
				tagCount := rapid.IntRange(1, 3).Draw(t, "exceptOnCount")
				tags := make([]string, tagCount)
				for j := range tags {
					tags[j] = genMachineTag().Draw(t, "exceptOnTag")
				}
				entries[i] = manifest.PackageEntry{Name: name, ExceptOn: tags}
			}
		}
		return entries
	})
}

// ---------------------------------------------------------------------------
// Property 3: Machine filter correctness
// Packages with only_on = [X] where machineTag ≠ X are excluded;
// packages with except_on = [X] where machineTag = X are excluded.
//
// **Validates: Requirements 3.1, 3.2, 3.3**
// ---------------------------------------------------------------------------

// TestPropertyFilterExcludesOnlyOnMismatch verifies that entries with only_on
// set to tags that do NOT include the machineTag are excluded from the result.
//
// **Validates: Requirements 3.1**
func TestPropertyFilterExcludesOnlyOnMismatch(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		machineTag := genMachineTag().Draw(rt, "machineTag")

		// Generate 1-3 tags that are all different from machineTag
		tagCount := rapid.IntRange(1, 3).Draw(rt, "tagCount")
		tags := make([]string, tagCount)
		for i := range tags {
			tags[i] = genMachineTagDistinctFrom(machineTag).Draw(rt, "otherTag")
		}

		// Generate 1-10 entries all with only_on = tags (none matching machineTag)
		entryCount := rapid.IntRange(1, 10).Draw(rt, "entryCount")
		entries := make([]manifest.PackageEntry, entryCount)
		for i := range entries {
			entries[i] = genPackageEntryWithOnlyOn(tags).Draw(rt, "entry")
		}

		result := FilterForMachine(entries, machineTag)

		if len(result) != 0 {
			rt.Fatalf("expected 0 entries when machineTag %q not in only_on %v, got %d",
				machineTag, tags, len(result))
		}
	})
}

// TestPropertyFilterExcludesExceptOnMatch verifies that entries with except_on
// containing the machineTag are excluded from the result.
//
// **Validates: Requirements 3.2**
func TestPropertyFilterExcludesExceptOnMatch(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		machineTag := genMachineTag().Draw(rt, "machineTag")

		// Build except_on list that always includes machineTag, plus optional extras
		extraCount := rapid.IntRange(0, 2).Draw(rt, "extraCount")
		tags := []string{machineTag}
		for i := 0; i < extraCount; i++ {
			tags = append(tags, genMachineTag().Draw(rt, "extraTag"))
		}

		// Generate 1-10 entries all with except_on containing machineTag
		entryCount := rapid.IntRange(1, 10).Draw(rt, "entryCount")
		entries := make([]manifest.PackageEntry, entryCount)
		for i := range entries {
			entries[i] = genPackageEntryWithExceptOn(tags).Draw(rt, "entry")
		}

		result := FilterForMachine(entries, machineTag)

		if len(result) != 0 {
			rt.Fatalf("expected 0 entries when machineTag %q is in except_on %v, got %d",
				machineTag, tags, len(result))
		}
	})
}

// TestPropertyFilterIncludesNoFilter verifies that entries with neither
// only_on nor except_on are always included in the result.
//
// **Validates: Requirements 3.3**
func TestPropertyFilterIncludesNoFilter(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		machineTag := genMachineTag().Draw(rt, "machineTag")

		// Generate 1-10 entries with no filters
		entryCount := rapid.IntRange(1, 10).Draw(rt, "entryCount")
		entries := make([]manifest.PackageEntry, entryCount)
		for i := range entries {
			entries[i] = genPackageEntryNoFilter().Draw(rt, "entry")
		}

		result := FilterForMachine(entries, machineTag)

		if len(result) != len(entries) {
			rt.Fatalf("expected all %d unfiltered entries to be included, got %d",
				len(entries), len(result))
		}
	})
}

// TestPropertyFilterIncludesOnlyOnMatch verifies that entries with only_on
// containing the machineTag are included in the result.
//
// **Validates: Requirements 3.1**
func TestPropertyFilterIncludesOnlyOnMatch(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		machineTag := genMachineTag().Draw(rt, "machineTag")

		// Build only_on list that always includes machineTag, plus optional extras
		extraCount := rapid.IntRange(0, 2).Draw(rt, "extraCount")
		tags := []string{machineTag}
		for i := 0; i < extraCount; i++ {
			tags = append(tags, genMachineTag().Draw(rt, "extraTag"))
		}

		// Generate 1-10 entries all with only_on containing machineTag
		entryCount := rapid.IntRange(1, 10).Draw(rt, "entryCount")
		entries := make([]manifest.PackageEntry, entryCount)
		for i := range entries {
			entries[i] = genPackageEntryWithOnlyOn(tags).Draw(rt, "entry")
		}

		result := FilterForMachine(entries, machineTag)

		if len(result) != len(entries) {
			rt.Fatalf("expected all %d entries with machineTag %q in only_on to be included, got %d",
				len(entries), machineTag, len(result))
		}
	})
}

// ---------------------------------------------------------------------------
// Property 8: Filter mutual exclusivity
// FilterForMachine never includes a package where machineTag is in ExceptOn,
// and never excludes a package where machineTag is in OnlyOn.
//
// **Validates: Requirements 3.1, 3.2, 3.3**
// ---------------------------------------------------------------------------

// TestPropertyFilterNeverIncludesExceptOnMatch verifies that for any random
// set of entries and machineTag, no entry in the result has machineTag in its
// ExceptOn list.
//
// **Validates: Requirements 3.2**
func TestPropertyFilterNeverIncludesExceptOnMatch(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		machineTag := genMachineTag().Draw(rt, "machineTag")
		entries := genMixedEntries().Draw(rt, "entries")

		result := FilterForMachine(entries, machineTag)

		for _, entry := range result {
			for _, tag := range entry.ExceptOn {
				if tag == machineTag {
					rt.Fatalf("filtered result includes entry %q with machineTag %q in ExceptOn %v",
						entry.Name, machineTag, entry.ExceptOn)
				}
			}
		}
	})
}

// TestPropertyFilterNeverExcludesOnlyOnMatch verifies that for any random set
// of entries and machineTag, every entry whose OnlyOn list contains machineTag
// appears in the result.
//
// **Validates: Requirements 3.1**
func TestPropertyFilterNeverExcludesOnlyOnMatch(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		machineTag := genMachineTag().Draw(rt, "machineTag")
		entries := genMixedEntries().Draw(rt, "entries")

		result := FilterForMachine(entries, machineTag)

		// Build a set of names in the result for quick lookup.
		// Since names may not be unique, track by index instead.
		resultSet := make(map[int]bool)
		for i := range result {
			// Find the original index by matching pointer-equivalent entry
			for j := range entries {
				if entries[j].Name == result[i].Name &&
					sliceEqual(entries[j].OnlyOn, result[i].OnlyOn) &&
					sliceEqual(entries[j].ExceptOn, result[i].ExceptOn) {
					resultSet[j] = true
					break
				}
			}
		}

		for i, entry := range entries {
			if len(entry.OnlyOn) > 0 && sliceContains(entry.OnlyOn, machineTag) {
				if !resultSet[i] {
					rt.Fatalf("entry %q (index %d) has machineTag %q in OnlyOn %v but was excluded from result",
						entry.Name, i, machineTag, entry.OnlyOn)
				}
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func sliceContains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
