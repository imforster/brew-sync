package diff

import "brew-sync/internal/manifest"

// DiffResult holds the classified packages after comparing manifest against local state.
type DiffResult struct {
	ToInstall []manifest.PackageEntry
	ToRemove  []manifest.PackageEntry
	ToUpgrade []manifest.PackageEntry
	Unchanged []manifest.PackageEntry
}

// LocalState represents the current Homebrew installation state on the local machine.
type LocalState struct {
	Formulae []Package
	Casks    []Package
	Taps     []string
}

// Package represents a locally installed Homebrew package.
type Package struct {
	Name    string
	Version string
}
