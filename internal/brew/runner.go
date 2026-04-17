package brew

import (
	"fmt"
	"os/exec"
	"strings"

	"brew-sync/internal/diff"
)

// BrewRunner abstracts Homebrew CLI operations for querying and mutating the local installation.
type BrewRunner interface {
	// ListFormulae returns all installed Homebrew formulae with their versions.
	ListFormulae() ([]diff.Package, error)
	// ListCasks returns all installed Homebrew casks with their versions.
	ListCasks() ([]diff.Package, error)
	// ListTaps returns all configured Homebrew taps.
	ListTaps() ([]string, error)
	// Install installs a package. If pkg.Version is set, installs that specific version.
	Install(pkg diff.Package) error
	// Uninstall removes a package.
	Uninstall(pkg diff.Package) error
	// Upgrade upgrades a package to the latest (or specified) version.
	Upgrade(pkg diff.Package) error
	// Update runs brew update to refresh Homebrew's package index.
	Update() error
	// IsInstalled returns true if the brew binary exists in PATH.
	IsInstalled() bool
}

// RealBrewRunner executes actual brew CLI commands via os/exec.
type RealBrewRunner struct{}

// NewRealBrewRunner creates a new RealBrewRunner.
func NewRealBrewRunner() *RealBrewRunner {
	return &RealBrewRunner{}
}

// IsInstalled checks whether the brew binary is available in PATH.
func (r *RealBrewRunner) IsInstalled() bool {
	_, err := exec.LookPath("brew")
	return err == nil
}

// ListFormulae runs `brew list --formula --versions` and parses the output.
func (r *RealBrewRunner) ListFormulae() ([]diff.Package, error) {
	cmd := exec.Command("brew", "list", "--formula", "--versions")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list formulae: %w", err)
	}
	return parseBrewListOutput(string(output)), nil
}

// ListCasks runs `brew list --cask --versions` and parses the output.
func (r *RealBrewRunner) ListCasks() ([]diff.Package, error) {
	cmd := exec.Command("brew", "list", "--cask", "--versions")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list casks: %w", err)
	}
	return parseBrewListOutput(string(output)), nil
}

// ListTaps runs `brew tap` and returns one tap per line.
func (r *RealBrewRunner) ListTaps() ([]string, error) {
	cmd := exec.Command("brew", "tap")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list taps: %w", err)
	}
	return parseLines(string(output)), nil
}

// Install runs `brew install <name>` or `brew install <name@version>` if a version is specified.
func (r *RealBrewRunner) Install(pkg diff.Package) error {
	name := pkg.Name
	if pkg.Version != "" {
		name = pkg.Name + "@" + pkg.Version
	}
	cmd := exec.Command("brew", "install", name)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to install %s: %s: %w", name, string(output), err)
	}
	return nil
}

// Uninstall runs `brew uninstall <name>`.
func (r *RealBrewRunner) Uninstall(pkg diff.Package) error {
	cmd := exec.Command("brew", "uninstall", pkg.Name)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to uninstall %s: %s: %w", pkg.Name, string(output), err)
	}
	return nil
}

// Upgrade runs `brew upgrade <name>`.
func (r *RealBrewRunner) Upgrade(pkg diff.Package) error {
	cmd := exec.Command("brew", "upgrade", pkg.Name)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to upgrade %s: %s: %w", pkg.Name, string(output), err)
	}
	return nil
}

// Update runs `brew update` to refresh the package index.
func (r *RealBrewRunner) Update() error {
	cmd := exec.Command("brew", "update")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to update brew: %s: %w", string(output), err)
	}
	return nil
}

// parseBrewListOutput parses the output of `brew list --formula/--cask --versions`.
// Each line has the format: "name version1 version2 ..." — we take the first version.
func parseBrewListOutput(output string) []diff.Package {
	var packages []diff.Package
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		pkg := diff.Package{
			Name: fields[0],
		}
		if len(fields) > 1 {
			pkg.Version = fields[1]
		}
		packages = append(packages, pkg)
	}
	return packages
}

// parseLines splits output into non-empty trimmed lines.
func parseLines(output string) []string {
	var lines []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
