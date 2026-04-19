package cmd

import (
	"fmt"

	"brew-sync/internal/brew"
	"brew-sync/internal/manifest"
	"brew-sync/internal/sync"

	"github.com/spf13/cobra"
)

var pushCmd = &cobra.Command{
	Use:   "push",
	Short: "Push the local manifest to a remote location",
	Long: `Snapshot the current Homebrew installation, build a manifest from the
local state, save it to the configured manifest path, and push it to the
configured remote sync backend.

If no configuration file is found, the manifest is saved locally only.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		runner := brew.NewRealBrewRunner()
		manager := manifest.NewManifestManager()

		// Step 1: Snapshot local brew state
		if verbose {
			fmt.Println("[verbose] Querying local Homebrew state...")
		}

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

		if verbose {
			fmt.Printf("[verbose] Local state: %d formulae, %d casks, %d taps\n", len(formulae), len(casks), len(taps))
		}

		// Step 2: Convert to LocalPackage slices
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

		// Step 3: Build manifest from local state
		if verbose {
			fmt.Println("[verbose] Building manifest from local state...")
		}

		cfg, cfgErr := loadConfig(GetConfigPath())
		machineTag := getMachineTag(cfg)
		updatedBy := getUpdatedBy()
		m := manager.BuildFromLocal(localFormulae, localCasks, taps, machineTag, updatedBy)

		// Step 4: Save manifest locally using configured path
		outputPath := getManifestPath(cfg)

		if verbose {
			fmt.Printf("[verbose] Saving manifest to %s\n", outputPath)
		}

		if err := manager.Save(outputPath, m); err != nil {
			return fmt.Errorf("failed to save manifest: %w", err)
		}

		fmt.Printf("Manifest saved to %s (%d formulae, %d casks, %d taps)\n",
			outputPath, len(m.Formulae), len(m.Casks), len(m.Taps))

		// Step 5: Try to push via sync backend (only if config loaded successfully)
		if cfgErr != nil {
			fmt.Println("No sync backend configured — manifest saved locally only.")
			fmt.Println("To push remotely, configure a sync backend in your config file.")
			return nil
		}

		backend, err := sync.NewSyncBackend(cfg)
		if err != nil {
			return fmt.Errorf("failed to create sync backend: %w", err)
		}

		if verbose {
			fmt.Printf("[verbose] Pushing manifest via %s backend...\n", backend.Name())
		}

		if err := backend.Push(outputPath); err != nil {
			return fmt.Errorf("failed to push manifest via %s backend: %w", backend.Name(), err)
		}

		fmt.Printf("Manifest pushed successfully via %s backend.\n", backend.Name())

		if verbose {
			fmt.Println("[verbose] Push completed successfully")
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(pushCmd)
}
