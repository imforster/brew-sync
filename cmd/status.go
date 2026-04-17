package cmd

import (
	"fmt"
	"os"

	"brew-sync/internal/brew"
	"brew-sync/internal/diff"
	"brew-sync/internal/manifest"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show drift between the manifest and local packages",
	Long: `Compare the brew-sync.toml manifest against the locally installed
Homebrew packages and display a human-readable summary of what would change.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := loadConfigGraceful()
		manager := manifest.NewManifestManager()

		// Load manifest from configured path
		manifestPath := getManifestPath(cfg)
		m, err := manager.Load(manifestPath)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("manifest not found at %s\nRun 'brew-sync init' to generate one from your local packages, or 'brew-sync pull' to fetch from a remote", manifestPath)
			}
			return fmt.Errorf("failed to load manifest: %w", err)
		}

		// Query local brew state
		runner := brew.NewRealBrewRunner()

		formulae, err := runner.ListFormulae()
		if err != nil {
			return fmt.Errorf("failed to list formulae: %w", err)
		}

		casks, err := runner.ListCasks()
		if err != nil {
			return fmt.Errorf("failed to list casks: %w", err)
		}

		// Build local state
		localState := &diff.LocalState{
			Formulae: formulae,
			Casks:    casks,
		}

		// Compute diff with machine tag from config
		result := diff.ComputeDiff(m, localState, getMachineTag(cfg))

		// Print human-readable summary
		printStatusSummary(result)

		return nil
	},
}

func printStatusSummary(result *diff.DiffResult) {
	installCount := len(result.ToInstall)
	removeCount := len(result.ToRemove)
	upgradeCount := len(result.ToUpgrade)
	unchangedCount := len(result.Unchanged)

	if installCount == 0 && removeCount == 0 && upgradeCount == 0 {
		fmt.Println("Everything is in sync!")
		return
	}

	fmt.Printf("%d packages to install\n", installCount)
	fmt.Printf("%d packages to upgrade\n", upgradeCount)
	if removeCount > 0 {
		fmt.Printf("%d local-only packages not in manifest\n", removeCount)
	}
	fmt.Printf("%d packages unchanged\n", unchangedCount)

	if verbose {
		if installCount > 0 {
			fmt.Println("\nTo install:")
			for _, pkg := range result.ToInstall {
				fmt.Printf("  + %s\n", pkg.Name)
			}
		}
		if upgradeCount > 0 {
			fmt.Println("\nTo upgrade:")
			for _, pkg := range result.ToUpgrade {
				if pkg.Version != "" {
					fmt.Printf("  ~ %s (-> %s)\n", pkg.Name, pkg.Version)
				} else {
					fmt.Printf("  ~ %s\n", pkg.Name)
				}
			}
		}
		if removeCount > 0 {
			fmt.Println("\nLocal-only (not in manifest):")
			for _, pkg := range result.ToRemove {
				fmt.Printf("  ? %s\n", pkg.Name)
			}
			fmt.Println("\n  Run 'brew-sync reconcile' to add these to the manifest,")
			fmt.Println("  or 'brew-sync apply --remove' to uninstall them.")
		}
	}
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
