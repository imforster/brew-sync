package cmd

import (
	"fmt"

	"brew-sync/internal/brew"
	"brew-sync/internal/manifest"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Generate a manifest from the current Homebrew installation",
	Long: `Generate a brew-sync.toml manifest by querying the locally installed
formulae, casks, and taps. This captures your current Homebrew state as the
starting point for synchronization.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := loadConfigGraceful()
		runner := brew.NewRealBrewRunner()
		manager := manifest.NewManifestManager()

		// Query local brew state
		formulae, err := runner.ListFormulae()
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

		// Convert diff.Package slices to manifest.LocalPackage slices
		localFormulae := make([]manifest.LocalPackage, len(formulae))
		for i, pkg := range formulae {
			localFormulae[i] = manifest.LocalPackage{
				Name:    pkg.Name,
				Version: pkg.Version,
			}
		}

		localCasks := make([]manifest.LocalPackage, len(casks))
		for i, pkg := range casks {
			localCasks[i] = manifest.LocalPackage{
				Name:    pkg.Name,
				Version: pkg.Version,
			}
		}

		// Build manifest from local state
		m := manager.BuildFromLocal(localFormulae, localCasks, taps)

		// Save manifest to configured path
		outputPath := getManifestPath(cfg)
		if err := manager.Save(outputPath, m); err != nil {
			return fmt.Errorf("failed to save manifest: %w", err)
		}

		fmt.Printf("Manifest written to %s (%d formulae, %d casks, %d taps)\n",
			outputPath, len(m.Formulae), len(m.Casks), len(m.Taps))

		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
