package manifest

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// ManifestManager handles loading, saving, and managing manifest files.
type ManifestManager struct{}

// NewManifestManager creates a new ManifestManager.
func NewManifestManager() *ManifestManager {
	return &ManifestManager{}
}

// Load reads a TOML manifest file from the given path and deserializes it into a Manifest.
func (mm *ManifestManager) Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var m Manifest
	if err := toml.Unmarshal(data, &m); err != nil {
		return nil, err
	}

	return &m, nil
}

// Save serializes the given Manifest to TOML and writes it to the specified path.
// Parent directories are created if they do not exist.
func (mm *ManifestManager) Save(path string, m *Manifest) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	var buf bytes.Buffer
	encoder := toml.NewEncoder(&buf)
	if err := encoder.Encode(m); err != nil {
		return err
	}

	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// LocalPackage represents a locally installed Homebrew package with a name and version.
// This type is used as input to BuildFromLocal to avoid circular imports with the diff package.
type LocalPackage struct {
	Name    string
	Version string
}

// AddMachineToList adds machineTag to the machines list if non-empty and not already present.
// Returns the updated sorted list.
func AddMachineToList(machines []string, machineTag string) []string {
	if machineTag == "" {
		return machines
	}
	for _, m := range machines {
		if m == machineTag {
			return machines
		}
	}
	machines = append(machines, machineTag)
	sort.Strings(machines)
	return machines
}

// BuildFromLocal creates a new Manifest from the locally installed Homebrew packages.
// It converts LocalPackage entries to PackageEntry (no machine filters), sorts
// formulae and casks by name, sorts taps alphabetically, sets version to 1,
// and records the current time, machine tag, and user in metadata.
func (mm *ManifestManager) BuildFromLocal(formulae []LocalPackage, casks []LocalPackage, taps []string, machineTag, updatedBy string) *Manifest {
	manifestFormulae := make([]PackageEntry, len(formulae))
	for i, pkg := range formulae {
		manifestFormulae[i] = PackageEntry{
			Name:    pkg.Name,
			Version: pkg.Version,
		}
	}
	sort.Slice(manifestFormulae, func(i, j int) bool {
		return manifestFormulae[i].Name < manifestFormulae[j].Name
	})

	manifestCasks := make([]PackageEntry, len(casks))
	for i, pkg := range casks {
		manifestCasks[i] = PackageEntry{
			Name:    pkg.Name,
			Version: pkg.Version,
		}
	}
	sort.Slice(manifestCasks, func(i, j int) bool {
		return manifestCasks[i].Name < manifestCasks[j].Name
	})

	sortedTaps := make([]string, len(taps))
	copy(sortedTaps, taps)
	sort.Strings(sortedTaps)

	return &Manifest{
		Version: 1,
		Metadata: ManifestMetadata{
			UpdatedAt: time.Now().UTC().Format(time.RFC3339),
			UpdatedBy: updatedBy,
			Machine:   machineTag,
			Machines:  AddMachineToList(nil, machineTag),
		},
		Formulae: manifestFormulae,
		Casks:    manifestCasks,
		Taps:     sortedTaps,
	}
}

// MergeLocal unions local packages into an existing manifest.
// It adds packages not already present and updates versions of existing packages
// to match local state. Packages in the manifest but not installed locally are preserved.
// Returns counts of added and version-updated packages.
func (mm *ManifestManager) MergeLocal(m *Manifest, localFormulae, localCasks []LocalPackage, taps []string, machineTag, updatedBy string) (added, updated int) {
	// Index existing manifest entries
	formulaeIdx := make(map[string]int, len(m.Formulae))
	for i, e := range m.Formulae {
		formulaeIdx[e.Name] = i
	}
	casksIdx := make(map[string]int, len(m.Casks))
	for i, e := range m.Casks {
		casksIdx[e.Name] = i
	}

	// Merge formulae
	for _, pkg := range localFormulae {
		if i, exists := formulaeIdx[pkg.Name]; exists {
			if pkg.Version != "" && m.Formulae[i].Version != pkg.Version {
				m.Formulae[i].Version = pkg.Version
				updated++
			}
		} else {
			m.Formulae = append(m.Formulae, PackageEntry{Name: pkg.Name, Version: pkg.Version})
			added++
		}
	}

	// Merge casks
	for _, pkg := range localCasks {
		if i, exists := casksIdx[pkg.Name]; exists {
			if pkg.Version != "" && m.Casks[i].Version != pkg.Version {
				m.Casks[i].Version = pkg.Version
				updated++
			}
		} else {
			m.Casks = append(m.Casks, PackageEntry{Name: pkg.Name, Version: pkg.Version})
			added++
		}
	}

	// Merge taps
	tapSet := make(map[string]bool, len(m.Taps))
	for _, t := range m.Taps {
		tapSet[t] = true
	}
	for _, t := range taps {
		if !tapSet[t] {
			m.Taps = append(m.Taps, t)
			added++
		}
	}

	// Sort everything
	sort.Slice(m.Formulae, func(i, j int) bool { return m.Formulae[i].Name < m.Formulae[j].Name })
	sort.Slice(m.Casks, func(i, j int) bool { return m.Casks[i].Name < m.Casks[j].Name })
	sort.Strings(m.Taps)

	// Update metadata
	m.Metadata.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	m.Metadata.UpdatedBy = updatedBy
	m.Metadata.Machine = machineTag
	m.Metadata.Machines = AddMachineToList(m.Metadata.Machines, machineTag)

	return added, updated
}

// Validate checks a manifest for structural correctness.
// It collects all validation errors and returns them together using errors.Join
// so the user can fix them in one pass.
func (mm *ManifestManager) Validate(m *Manifest) error {
	var errs []error

	// Step 1: Version check
	if m.Version != 1 {
		errs = append(errs, fmt.Errorf("unsupported manifest version: %d", m.Version))
	}

	// Step 2: Duplicate and empty name check within formulae
	seenFormulae := make(map[string]bool)
	for _, entry := range m.Formulae {
		if entry.Name == "" {
			errs = append(errs, fmt.Errorf("formula entry has empty name"))
			continue
		}
		if seenFormulae[entry.Name] {
			errs = append(errs, fmt.Errorf("duplicate formula: %s", entry.Name))
		}
		seenFormulae[entry.Name] = true
	}

	// Duplicate and empty name check within casks
	seenCasks := make(map[string]bool)
	for _, entry := range m.Casks {
		if entry.Name == "" {
			errs = append(errs, fmt.Errorf("cask entry has empty name"))
			continue
		}
		if seenCasks[entry.Name] {
			errs = append(errs, fmt.Errorf("duplicate cask: %s", entry.Name))
		}
		seenCasks[entry.Name] = true
	}

	// Step 3: Mutual exclusivity of only_on / except_on
	allEntries := make([]PackageEntry, 0, len(m.Formulae)+len(m.Casks))
	allEntries = append(allEntries, m.Formulae...)
	allEntries = append(allEntries, m.Casks...)
	for _, entry := range allEntries {
		if len(entry.OnlyOn) > 0 && len(entry.ExceptOn) > 0 {
			errs = append(errs, fmt.Errorf("package %s has both only_on and except_on set", entry.Name))
		}
	}

	// Step 4: Tap format validation
	for _, tap := range m.Taps {
		if !isValidTapFormat(tap) {
			errs = append(errs, fmt.Errorf("invalid tap format: %s (expected owner/repo)", tap))
		}
	}

	return errors.Join(errs...)
}

// isValidTapFormat checks that a tap string matches the owner/repo format:
// exactly one "/" with non-empty parts on both sides.
func isValidTapFormat(tap string) bool {
	parts := strings.SplitN(tap, "/", 3)
	if len(parts) != 2 {
		return false
	}
	return parts[0] != "" && parts[1] != ""
}
