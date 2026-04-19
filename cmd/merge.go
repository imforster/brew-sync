package cmd

import (
	"fmt"
	"os"

	"brew-sync/internal/brew"
	"brew-sync/internal/manifest"

	"github.com/spf13/cobra"
)

var mergeCmd = &cobra.Command{
	Use:   "merge",
	Short: "Merge local packages into the existing manifest",
	Long: `Union the local Homebrew state into the existing manifest. Adds packages
not already present and updates versions of existing packages to match local
state. Packages in the manifest but not installed locally are preserved
(they belong to other machines).

This is the non-interactive equivalent of running reconcile and choosing "add"
for every local-only package, plus updating all version drift.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := loadConfigGraceful()
		manager := manifest.NewManifestManager()

		manifestPath := getManifestPath(cfg)
		m, err := manager.Load(manifestPath)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("manifest not found at %s\nRun 'brew-sync init' first", manifestPath)
			}
			return fmt.Errorf("failed to load manifest: %w", err)
		}

		runner := brew.NewRealBrewRunner()

		formulae, err := runner.ListLeaves()
		if err != nil {
			return fmt.Errorf("failed to list formulae: %w", err)
		}

		casks, err := runner.ListCasks()
		if err != nil {
			return fmt.Errorf("failed to list casks: %w", err)
		}

		taps, err := runner.ListTaps()
		if err != nil {
			return fmt.Errorf("failed to list taps: %w", err)
		}

		// Convert to manifest.LocalPackage
		localFormulae := make([]manifest.LocalPackage, len(formulae))
		for i, pkg := range formulae {
			localFormulae[i] = manifest.LocalPackage{Name: pkg.Name, Version: pkg.Version}
		}
		localCasks := make([]manifest.LocalPackage, len(casks))
		for i, pkg := range casks {
			localCasks[i] = manifest.LocalPackage{Name: pkg.Name, Version: pkg.Version}
		}

		machineTag := getMachineTag(cfg)
		updatedBy := getUpdatedBy()
		added, updated := manager.MergeLocal(m, localFormulae, localCasks, taps, machineTag, updatedBy)

		if err := manager.Save(manifestPath, m); err != nil {
			return fmt.Errorf("failed to save manifest: %w", err)
		}

		fmt.Printf("Manifest merged: %d added, %d versions updated (%d formulae, %d casks, %d taps)\n",
			added, updated, len(m.Formulae), len(m.Casks), len(m.Taps))

		return nil
	},
}

func init() {
	rootCmd.AddCommand(mergeCmd)
}
