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
	// ListLeaves returns only explicitly installed formulae (not auto-installed dependencies).
	// These are packages the user installed directly; dependencies are pulled in automatically.
	ListLeaves() ([]diff.Package, error)
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

// ListLeaves runs `brew leaves` to get explicitly installed formulae, then
// cross-references with `brew list --formula --versions` to attach version info.
// This returns only top-level packages (not auto-installed dependencies).
func (r *RealBrewRunner) ListLeaves() ([]diff.Package, error) {
	// Get leaf names
	leavesCmd := exec.Command("brew", "leaves")
	leavesOutput, err := leavesCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list leaves: %w", err)
	}
	leafNames := make(map[string]bool)
	for _, name := range parseLines(string(leavesOutput)) {
		// brew leaves returns tap-prefixed names for third-party packages
		// (e.g. "cockroachdb/tap/cockroach") but brew list returns short names
		// (e.g. "cockroach"). Store both forms so the lookup works either way.
		leafNames[name] = true
		if i := strings.LastIndex(name, "/"); i >= 0 {
			leafNames[name[i+1:]] = true
		}
	}

	// Get all formulae with versions
	allFormulae, err := r.ListFormulae()
	if err != nil {
		return nil, err
	}

	// Filter to only leaves
	var leaves []diff.Package
	for _, pkg := range allFormulae {
		if leafNames[pkg.Name] {
			leaves = append(leaves, pkg)
		}
	}
	return leaves, nil
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

// Install runs `brew install <name>`. The version field is informational only —
// Homebrew does not support installing arbitrary versions via name@version syntax.
// Only formulae published with @ in their name (e.g. python@3.13) use that format,
// and those already have @ as part of pkg.Name.
func (r *RealBrewRunner) Install(pkg diff.Package) error {
	cmd := exec.Command("brew", "install", pkg.Name)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to install %s: %s: %w", pkg.Name, string(output), err)
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
