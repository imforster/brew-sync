package cmd

import (
	"fmt"
	"os"

	"brew-sync/internal/brew"
	"brew-sync/internal/diff"
	"brew-sync/internal/manifest"

	"github.com/spf13/cobra"
)

var (
	removeFlag    bool
	noInstallFlag bool
)

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply the manifest diff to the local machine",
	Long: `Apply the brew-sync.toml manifest to the local Homebrew installation.
Installs missing packages and upgrades outdated packages.

By default, packages installed locally but not in the manifest are kept.
Use --remove to also uninstall packages not in the manifest.
Use --dry-run to preview changes without applying them.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := loadConfigGraceful()
		manager := manifest.NewManifestManager()

		// Load manifest from configured path
		manifestPath := getManifestPath(cfg)
		if verbose {
			fmt.Printf("[verbose] Loading manifest from %s\n", manifestPath)
			machineTag := getMachineTag(cfg)
			if machineTag != "" {
				fmt.Printf("[verbose] Machine tag: %s\n", machineTag)
			} else {
				fmt.Println("[verbose] Machine tag: (none)")
			}
		}

		m, err := manager.Load(manifestPath)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("manifest not found at %s\nRun 'brew-sync init' to generate one from your local packages, or 'brew-sync pull' to fetch from a remote", manifestPath)
			}
			return fmt.Errorf("failed to load manifest: %w", err)
		}

		if verbose {
			fmt.Printf("[verbose] Manifest loaded: %d formulae, %d casks, %d taps\n",
				len(m.Formulae), len(m.Casks), len(m.Taps))
		}

		// Query local brew state
		runner := brew.NewRealBrewRunner()

		if verbose {
			fmt.Println("[verbose] Querying local Homebrew state...")
		}

		formulae, err := runner.ListFormulae()
		if err != nil {
			return fmt.Errorf("failed to list formulae: %w", err)
		}

		casks, err := runner.ListCasks()
		if err != nil {
			return fmt.Errorf("failed to list casks: %w", err)
		}

		if verbose {
			fmt.Printf("[verbose] Local state: %d formulae, %d casks\n", len(formulae), len(casks))
		}

		// Build local state
		localState := &diff.LocalState{
			Formulae: formulae,
			Casks:    casks,
		}

		// Apply missing taps before packages so tap-prefixed formulae can be installed.
		if !dryRun {
			localTaps, err := runner.ListTaps()
			if err != nil {
				return fmt.Errorf("failed to list taps: %w", err)
			}
			localTapSet := make(map[string]bool, len(localTaps))
			for _, t := range localTaps {
				localTapSet[t] = true
			}
			for _, tap := range m.Taps {
				if !localTapSet[tap] {
					fmt.Printf("  → tapping %s...\n", tap)
					if err := runner.Tap(tap); err != nil {
						fmt.Printf("  ✗ tap %s: %v\n", tap, err)
					} else {
						fmt.Printf("  ✓ tap %s\n", tap)
					}
				}
			}
		}

		// Compute diff with machine tag from config
		result := diff.ComputeDiff(m, localState, getMachineTag(cfg))

		if verbose {
			fmt.Printf("[verbose] Diff computed: %d to install, %d to upgrade, %d to remove, %d unchanged\n",
				len(result.ToInstall), len(result.ToUpgrade), len(result.ToRemove), len(result.Unchanged))
		}

		// Apply diff (RealBrewRunner satisfies diff.Runner)
		skipRemove := !removeFlag
		if verbose && !dryRun {
			if noInstallFlag {
				fmt.Printf("[verbose] Skipping installation of %d packages (--no-install)\n", len(result.ToInstall))
			} else {
				for _, pkg := range result.ToInstall {
					fmt.Printf("[verbose] Installing %s...\n", pkg.Name)
				}
			}
			for _, pkg := range result.ToUpgrade {
				fmt.Printf("[verbose] Upgrading %s...\n", pkg.Name)
			}
			if skipRemove && len(result.ToRemove) > 0 {
				fmt.Printf("[verbose] Skipping removal of %d local-only packages (use --remove to uninstall)\n", len(result.ToRemove))
			}
			if !skipRemove {
				for _, pkg := range result.ToRemove {
					fmt.Printf("[verbose] Removing %s...\n", pkg.Name)
				}
			}
		}

		// Print progress before each operation
		progressCb := func(operation, pkgName string) {
			fmt.Printf("  → %s %s...\n", operation, pkgName)
		}

		report, applyErr := diff.ApplyDiff(result, runner, dryRun, diff.ApplyOptions{SkipRemove: skipRemove, SkipInstall: noInstallFlag, OnProgress: progressCb})

		// Update manifest versions for successful upgrades so next run is idempotent
		if !dryRun {
			var upgraded []string
			for _, r := range report.Results {
				if r.Err == nil && r.Operation == "upgrade" {
					upgraded = append(upgraded, r.Package)
				}
			}
			if len(upgraded) > 0 {
				freshFormulae, _ := runner.ListFormulae()
				freshCasks, _ := runner.ListCasks()
				freshVersions := make(map[string]string)
				for _, p := range freshFormulae {
					freshVersions[p.Name] = p.Version
				}
				for _, p := range freshCasks {
					freshVersions[p.Name] = p.Version
				}
				dirty := false
				for _, name := range upgraded {
					if v, ok := freshVersions[name]; ok {
						if markPackage(m, name, func(e *manifest.PackageEntry) { e.Version = v }) {
							dirty = true
						}
					}
				}
				if dirty {
					if err := manager.Save(manifestPath, m); err != nil {
						fmt.Fprintf(os.Stderr, "warning: failed to save updated versions to manifest: %v\n", err)
						fmt.Fprintln(os.Stderr, "         Next run may re-attempt the same upgrades.")
					} else {
						fmt.Println("  ✓ manifest updated")
					}
				}
			}
		}

		// Print report
		printApplyReport(report)

		if verbose {
			if applyErr != nil {
				fmt.Printf("[verbose] Apply finished with errors: %v\n", applyErr)
			} else {
				fmt.Println("[verbose] Apply finished successfully")
			}
		}

		// Return the apply error (if any) to trigger non-zero exit
		return applyErr
	},
}

// markPackage finds a package by name in formulae or casks and applies the mutator.
// Returns true if the package was found and modified.
func markPackage(m *manifest.Manifest, name string, mutate func(*manifest.PackageEntry)) bool {
	for i := range m.Formulae {
		if m.Formulae[i].Name == name {
			mutate(&m.Formulae[i])
			return true
		}
	}
	for i := range m.Casks {
		if m.Casks[i].Name == name {
			mutate(&m.Casks[i])
			return true
		}
	}
	return false
}

func printApplyReport(report *diff.ApplyReport) {
	if report.Planned {
		// Dry-run mode
		fmt.Printf("Would install %d, upgrade %d packages\n",
			report.InstallCount, report.UpgradeCount)
		if report.RemoveCount > 0 {
			fmt.Printf("Would remove %d packages\n", report.RemoveCount)
		}
		if report.SkippedRemoveCount > 0 {
			fmt.Printf("%d local-only packages kept (use --remove to uninstall)\n", report.SkippedRemoveCount)
		}
		return
	}

	// Normal mode: print each result
	for _, r := range report.Results {
		if r.Err != nil {
			fmt.Printf("  ✗ %s %s: %v\n", r.Operation, r.Package, r.Err)
		} else {
			fmt.Printf("  ✓ %s %s\n", r.Operation, r.Package)
		}
	}

	// Summary
	total := len(report.Results)
	failed := report.ErrorCount
	succeeded := total - failed
	fmt.Printf("\n%d succeeded, %d failed out of %d operations\n", succeeded, failed, total)

	if report.SkippedRemoveCount > 0 {
		fmt.Printf("%d local-only packages kept (use --remove to uninstall)\n", report.SkippedRemoveCount)
	}
}

func init() {
	applyCmd.Flags().BoolVar(&removeFlag, "remove", false, "also uninstall packages not in the manifest")
	applyCmd.Flags().BoolVar(&noInstallFlag, "no-install", false, "skip installing missing packages (upgrades only)")
	rootCmd.AddCommand(applyCmd)
}
