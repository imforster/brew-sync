package diff

import "brew-sync/internal/manifest"

// FilterForMachine returns the subset of entries applicable to the given machine tag.
// If an entry has OnlyOn set, it is included only if machineTag is in the list.
// If an entry has ExceptOn set, it is included only if machineTag is NOT in the list.
// If neither is set, the entry is always included.
// The original slice is not modified.
func FilterForMachine(entries []manifest.PackageEntry, machineTag string) []manifest.PackageEntry {
	result := make([]manifest.PackageEntry, 0, len(entries))
	for _, entry := range entries {
		switch {
		case len(entry.OnlyOn) > 0:
			if contains(entry.OnlyOn, machineTag) {
				result = append(result, entry)
			}
		case len(entry.ExceptOn) > 0:
			if !contains(entry.ExceptOn, machineTag) {
				result = append(result, entry)
			}
		default:
			result = append(result, entry)
		}
	}
	return result
}

// contains reports whether s is present in the slice.
func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
