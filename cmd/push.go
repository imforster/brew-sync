package cmd

import (
	"fmt"
	"os"

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

		cfg, cfgErr := loadConfig(GetConfigPath())
		machineTag := getMachineTag(cfg)
		updatedBy := getUpdatedBy()
		outputPath := getManifestPath(cfg)

		// Step 3: Create sync backend early so we can pull before building the manifest.
		var backend sync.SyncBackend
		if cfgErr == nil {
			b, err := sync.NewSyncBackend(cfg)
			if err != nil {
				return fmt.Errorf("failed to create sync backend: %w", err)
			}
			backend = b
		}

		// Step 4: Build manifest — merge local state into the existing remote manifest
		// to preserve entries from other machines. Fall back to a fresh build if no
		// remote manifest exists yet (first push).
		var m *manifest.Manifest
		if backend != nil {
			tmpPath := outputPath + ".tmp"
			if pullErr := backend.Pull(tmpPath); pullErr == nil {
				existing, loadErr := manager.Load(tmpPath)
				os.Remove(tmpPath)
				if loadErr == nil {
					manager.MergeLocal(existing, localFormulae, localCasks, taps, machineTag, updatedBy)
					m = existing
					if verbose {
						fmt.Println("[verbose] Merged local state into existing remote manifest")
					}
				}
			}
		}
		if m == nil {
			if verbose {
				fmt.Println("[verbose] Building new manifest from local state")
			}
			m = manager.BuildFromLocal(localFormulae, localCasks, taps, machineTag, updatedBy)
		}

		// Step 5: Save manifest locally using configured path
		if verbose {
			fmt.Printf("[verbose] Saving manifest to %s\n", outputPath)
		}

		if err := manager.Save(outputPath, m); err != nil {
			return fmt.Errorf("failed to save manifest: %w", err)
		}

		fmt.Printf("Manifest saved to %s (%d formulae, %d casks, %d taps)\n",
			outputPath, len(m.Formulae), len(m.Casks), len(m.Taps))

		// Step 6: Push via sync backend (only if config loaded successfully)
		if backend == nil {
			fmt.Println("No sync backend configured — manifest saved locally only.")
			fmt.Println("To push remotely, configure a sync backend in your config file.")
			return nil
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
