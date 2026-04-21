package cmd

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"brew-sync/internal/brew"
	"brew-sync/internal/diff"
	"brew-sync/internal/manifest"

	"github.com/spf13/cobra"
)

var reconcileCmd = &cobra.Command{
	Use:   "reconcile",
	Short: "Add local-only packages to the manifest",
	Long: `Walk through packages installed locally but not in the manifest and
choose what to do with each one:

  - Add to manifest for all machines
  - Add to manifest only for this machine (only_on)
  - Skip (leave installed but don't add to manifest)

This is useful after pulling a manifest from another machine to incorporate
packages that are unique to this machine.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := loadConfigGraceful()
		manager := manifest.NewManifestManager()

		// Load manifest
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

		formulae, err := runner.ListLeaves()
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

		// Compute diff
		machineTag := getMachineTag(cfg)
		result := diff.ComputeDiff(m, localState, machineTag)

		if len(result.ToRemove) == 0 {
			fmt.Println("No local-only packages found. Manifest and local state are aligned.")
			return nil
		}

		// Build sets of local formulae/cask names for type detection
		localFormulaeNames := make(map[string]bool)
		for _, pkg := range formulae {
			localFormulaeNames[pkg.Name] = true
		}

		fmt.Printf("Found %d local-only packages not in the manifest.\n", len(result.ToRemove))
		if machineTag != "" {
			fmt.Printf("Machine tag: %s\n", machineTag)
		}
		fmt.Println()

		reader := bufio.NewReader(os.Stdin)
		added := 0
		addedLocal := 0
		skipped := 0

		for _, pkg := range result.ToRemove {
			pkgType := "cask"
			if localFormulaeNames[pkg.Name] {
				pkgType = "formula"
			}

			fmt.Printf("  %s (%s)\n", pkg.Name, pkgType)
			fmt.Printf("    [a] Add to manifest (all machines)\n")
			if machineTag != "" {
				fmt.Printf("    [m] Add to manifest (only on %s)\n", machineTag)
			}
			fmt.Printf("    [s] Skip\n")
			fmt.Printf("    Choice: ")

			input, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("failed to read input: %w", err)
			}
			choice := strings.TrimSpace(strings.ToLower(input))

			entry := manifest.PackageEntry{Name: pkg.Name}

			switch choice {
			case "a":
				if pkgType == "formula" {
					m.Formulae = append(m.Formulae, entry)
				} else {
					m.Casks = append(m.Casks, entry)
				}
				added++
				fmt.Printf("    → Added %s for all machines\n\n", pkg.Name)

			case "m":
				if machineTag == "" {
					fmt.Println("    No machine_tag configured. Set machine_tag in your config to use this option.")
					fmt.Println("    Skipping.")
					skipped++
				} else {
					entry.OnlyOn = []string{machineTag}
					if pkgType == "formula" {
						m.Formulae = append(m.Formulae, entry)
					} else {
						m.Casks = append(m.Casks, entry)
					}
					addedLocal++
					fmt.Printf("    → Added %s (only_on: %s)\n\n", pkg.Name, machineTag)
				}

			default:
				skipped++
				fmt.Printf("    → Skipped\n\n")
			}
		}

		// Save updated manifest
		if added+addedLocal > 0 {
			sort.Slice(m.Formulae, func(i, j int) bool { return m.Formulae[i].Name < m.Formulae[j].Name })
			sort.Slice(m.Casks, func(i, j int) bool { return m.Casks[i].Name < m.Casks[j].Name })
			m.Metadata.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			m.Metadata.UpdatedBy = getUpdatedBy()
			m.Metadata.Machine = machineTag
			m.Metadata.Machines = manifest.AddMachineToList(m.Metadata.Machines, machineTag)
			if err := manager.Save(manifestPath, m); err != nil {
				return fmt.Errorf("failed to save manifest: %w", err)
			}
			fmt.Printf("Manifest updated: %d added for all machines, %d added for this machine only, %d skipped\n", added, addedLocal, skipped)
		} else {
			fmt.Printf("No changes made. %d packages skipped.\n", skipped)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(reconcileCmd)
}
