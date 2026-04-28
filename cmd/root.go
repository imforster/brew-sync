package cmd

import (
	"fmt"
	"os"
	"os/user"

	"brew-sync/internal/brew"
	"brew-sync/internal/config"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"
)

// Version info set via ldflags at build time.
var (
	Version = "dev"
	Commit  = "unknown"
)

var (
	// Global flag values accessible by subcommands.
	cfgFile string
	verbose bool
	dryRun  bool
)

// defaultManifestPath is the fallback manifest path when no config is loaded
// or the config does not specify a manifest_path.
const defaultManifestPath = "brew-sync.toml"

// GetConfigPath returns the value of the --config flag.
func GetConfigPath() string {
	return cfgFile
}

// GetVerbose returns the value of the --verbose flag.
func GetVerbose() bool {
	return verbose
}

// GetDryRun returns the value of the --dry-run flag.
func GetDryRun() bool {
	return dryRun
}

// loadConfig attempts to load the brew-sync configuration from the given path.
// Returns an error if the file does not exist or cannot be parsed.
func loadConfig(path string) (*config.Config, error) {
	// Expand ~ to home directory
	if len(path) > 0 && path[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to resolve home directory: %w", err)
		}
		path = home + path[1:]
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg config.Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &cfg, nil
}

// loadConfigGraceful loads config from the --config flag path.
// If the config file does not exist, it returns a zero-value Config (no error).
// This allows commands to work with defaults when no config is present.
func loadConfigGraceful() *config.Config {
	cfg, err := loadConfig(GetConfigPath())
	if err != nil {
		return &config.Config{}
	}
	return cfg
}

// getManifestPath returns the manifest path from config if set,
// otherwise returns the default "brew-sync.toml".
// Expands a leading ~ to the user's home directory.
func getManifestPath(cfg *config.Config) string {
	path := defaultManifestPath
	if cfg != nil && cfg.ManifestPath != "" {
		path = cfg.ManifestPath
	}
	if len(path) > 0 && path[0] == '~' {
		if home, err := os.UserHomeDir(); err == nil {
			path = home + path[1:]
		}
	}
	return path
}

// getMachineTag returns the machine tag from config if set,
// otherwise returns an empty string.
func getMachineTag(cfg *config.Config) string {
	if cfg != nil {
		return cfg.MachineTag
	}
	return ""
}
// getUpdatedBy returns the current OS username for manifest metadata.
func getUpdatedBy() string {
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return ""
}

var rootCmd = &cobra.Command{
	Use:     "brew-sync",
	Short:   "Synchronize Homebrew packages across machines",
	Version: Version + " (" + Commit + ")",
	Long: `brew-sync is a CLI tool that wraps Homebrew to synchronize installed
packages (formulae and casks) across multiple machines.

It uses a declarative TOML manifest to describe the desired set of packages.
The tool detects drift between the manifest and the local Homebrew installation,
then applies changes to converge the local state to the declared state.

Commands:
  init       Generate a manifest from the current Homebrew installation
  status     Show drift between the manifest and local packages
  apply      Apply the manifest diff to the local machine
  reconcile  Add local-only packages to the manifest interactively
  push       Push the local manifest to a remote location
  pull       Pull the shared manifest from a remote location`,
	SilenceUsage: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		runner := brew.NewRealBrewRunner()
		if !runner.IsInstalled() {
			return fmt.Errorf("brew not found. Please install Homebrew first: https://brew.sh")
		}
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "~/.config/brew-sync/config.toml", "path to config file")
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "enable verbose output")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "preview changes without applying")
}

// Execute runs the root command. This is the main entry point for the CLI.
func Execute() error {
	return rootCmd.Execute()
}
