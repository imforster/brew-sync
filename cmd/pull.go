package cmd

import (
	"fmt"

	"brew-sync/internal/sync"

	"github.com/spf13/cobra"
)

var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Pull the shared manifest from a remote location",
	Long: `Fetch the shared manifest from the configured remote sync backend
and write it to the local manifest path (brew-sync.toml).

This allows the current machine to synchronize to the latest declared
state. After pulling, run 'brew-sync status' to see what would change,
or 'brew-sync apply' to converge the local Homebrew installation.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Step 1: Load config
		if verbose {
			fmt.Printf("[verbose] Loading config from %s\n", GetConfigPath())
		}

		cfg, err := loadConfig(GetConfigPath())
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		// Step 2: Create sync backend from config
		backend, err := sync.NewSyncBackend(cfg)
		if err != nil {
			return fmt.Errorf("failed to create sync backend: %w", err)
		}

		if verbose {
			fmt.Printf("[verbose] Using %s sync backend\n", backend.Name())
		}

		// Step 3: Pull manifest from remote to configured path
		dest := getManifestPath(cfg)

		if verbose {
			fmt.Printf("[verbose] Pulling manifest to %s...\n", dest)
		}

		if err := backend.Pull(dest); err != nil {
			return fmt.Errorf("failed to pull manifest via %s backend: %w", backend.Name(), err)
		}

		// Step 4: Print confirmation
		fmt.Printf("Manifest pulled successfully via %s backend and saved to %s.\n", backend.Name(), dest)

		if verbose {
			fmt.Println("[verbose] Pull completed successfully")
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(pullCmd)
}
