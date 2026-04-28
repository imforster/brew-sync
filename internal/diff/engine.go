package diff

import "brew-sync/internal/manifest"

// ComputeDiff determines what actions are needed to converge local state to manifest.
//
// Algorithm:
//  1. Filter manifest entries for the current machine using FilterForMachine
//  2. Build local lookup maps (name → version) for O(1) membership checks
//  3. Walk manifest entries: classify as ToInstall, ToUpgrade, or Unchanged
//  4. Walk local entries: classify remaining as ToRemove
//
// Time complexity: O(m + l) where m = manifest entries, l = local entries
// Space complexity: O(m + l) for the lookup maps
//
// Loop invariant: at each iteration over manifest entries, all previously
// classified packages are in exactly one result set.
func ComputeDiff(m *manifest.Manifest, local *LocalState, machineTag string) *DiffResult {
	result := &DiffResult{}

	// Step 1: Filter manifest packages for this machine
	formulae := FilterForMachine(m.Formulae, machineTag)
	casks := FilterForMachine(m.Casks, machineTag)

	// Step 2: Build local lookup maps  name -> version
	localFormulaeMap := make(map[string]string, len(local.Formulae))
	for _, pkg := range local.Formulae {
		localFormulaeMap[pkg.Name] = pkg.Version
	}
	localCasksMap := make(map[string]string, len(local.Casks))
	for _, pkg := range local.Casks {
		localCasksMap[pkg.Name] = pkg.Version
	}

	// Step 3: Walk manifest entries
	// Invariant: each manifest entry is classified into exactly one of
	//            {ToInstall, ToUpgrade, Unchanged}
	seenFormulae := make(map[string]bool)
	seenCasks := make(map[string]bool)

	for _, entry := range formulae {
		seenFormulae[entry.Name] = true
		localVersion, exists := localFormulaeMap[entry.Name]
		if !exists {
			if entry.Deprecated || entry.Obsolete {
				result.Skipped = append(result.Skipped, entry)
			} else {
				result.ToInstall = append(result.ToInstall, entry)
			}
		} else if entry.Version != "" && entry.Version != localVersion {
			result.ToUpgrade = append(result.ToUpgrade, entry)
		} else {
			result.Unchanged = append(result.Unchanged, entry)
		}
	}

	for _, entry := range casks {
		seenCasks[entry.Name] = true
		localVersion, exists := localCasksMap[entry.Name]
		if !exists {
			if entry.Deprecated || entry.Obsolete {
				result.Skipped = append(result.Skipped, entry)
			} else {
				result.ToInstall = append(result.ToInstall, entry)
			}
		} else if entry.Version != "" && entry.Version != localVersion {
			result.ToUpgrade = append(result.ToUpgrade, entry)
		} else {
			result.Unchanged = append(result.Unchanged, entry)
		}
	}

	// Step 4: Walk local entries — anything not in manifest is a removal candidate
	// Invariant: each local entry not in seenFormulae/seenCasks is classified as ToRemove
	for _, pkg := range local.Formulae {
		if !seenFormulae[pkg.Name] {
			result.ToRemove = append(result.ToRemove, manifest.PackageEntry{Name: pkg.Name})
		}
	}
	for _, pkg := range local.Casks {
		if !seenCasks[pkg.Name] {
			result.ToRemove = append(result.ToRemove, manifest.PackageEntry{Name: pkg.Name})
		}
	}

	return result
}
