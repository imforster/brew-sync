package brew

import (
	"brew-sync/internal/diff"
)

// MockCall records a single operation performed on the MockBrewRunner.
type MockCall struct {
	Operation string
	Package   string
}

// MockBrewRunner implements BrewRunner for testing. It records all calls,
// tracks call counts, and returns configurable results and errors.
type MockBrewRunner struct {
	// Return values for list operations.
	Formulae []diff.Package
	Casks    []diff.Package
	Taps     []string

	// Return value for IsInstalled.
	Installed bool

	// Call counts for mutating operations.
	InstallCalls   int
	UninstallCalls int
	UpgradeCalls   int
	UpdateCalls    int

	// Ordered record of all calls made.
	Calls []MockCall

	// Default errors returned by each operation.
	ListFormulaeErr error
	ListCasksErr    error
	ListTapsErr     error
	InstallErr      error
	UninstallErr    error
	UpgradeErr      error
	UpdateErr       error

	// Per-package error overrides keyed by package name.
	// If a key is present, its error takes precedence over the default.
	InstallErrors   map[string]error
	UninstallErrors map[string]error
	UpgradeErrors   map[string]error
}

// NewMockBrewRunner creates a MockBrewRunner with sensible defaults.
func NewMockBrewRunner() *MockBrewRunner {
	return &MockBrewRunner{
		Installed:       true,
		InstallErrors:   make(map[string]error),
		UninstallErrors: make(map[string]error),
		UpgradeErrors:   make(map[string]error),
	}
}

// ListFormulae returns the configured formulae list or error.
func (m *MockBrewRunner) ListFormulae() ([]diff.Package, error) {
	m.Calls = append(m.Calls, MockCall{Operation: "list_formulae"})
	if m.ListFormulaeErr != nil {
		return nil, m.ListFormulaeErr
	}
	return m.Formulae, nil
}

// ListCasks returns the configured casks list or error.
func (m *MockBrewRunner) ListCasks() ([]diff.Package, error) {
	m.Calls = append(m.Calls, MockCall{Operation: "list_casks"})
	if m.ListCasksErr != nil {
		return nil, m.ListCasksErr
	}
	return m.Casks, nil
}

// ListTaps returns the configured taps list or error.
func (m *MockBrewRunner) ListTaps() ([]string, error) {
	m.Calls = append(m.Calls, MockCall{Operation: "list_taps"})
	if m.ListTapsErr != nil {
		return nil, m.ListTapsErr
	}
	return m.Taps, nil
}

// Install records the call and returns the per-package error if set, otherwise the default.
func (m *MockBrewRunner) Install(pkg diff.Package) error {
	m.InstallCalls++
	m.Calls = append(m.Calls, MockCall{Operation: "install", Package: pkg.Name})
	if err, ok := m.InstallErrors[pkg.Name]; ok {
		return err
	}
	return m.InstallErr
}

// Uninstall records the call and returns the per-package error if set, otherwise the default.
func (m *MockBrewRunner) Uninstall(pkg diff.Package) error {
	m.UninstallCalls++
	m.Calls = append(m.Calls, MockCall{Operation: "uninstall", Package: pkg.Name})
	if err, ok := m.UninstallErrors[pkg.Name]; ok {
		return err
	}
	return m.UninstallErr
}

// Upgrade records the call and returns the per-package error if set, otherwise the default.
func (m *MockBrewRunner) Upgrade(pkg diff.Package) error {
	m.UpgradeCalls++
	m.Calls = append(m.Calls, MockCall{Operation: "upgrade", Package: pkg.Name})
	if err, ok := m.UpgradeErrors[pkg.Name]; ok {
		return err
	}
	return m.UpgradeErr
}

// Update records the call and returns the configured error.
func (m *MockBrewRunner) Update() error {
	m.UpdateCalls++
	m.Calls = append(m.Calls, MockCall{Operation: "update"})
	return m.UpdateErr
}

// IsInstalled returns the configured Installed value.
func (m *MockBrewRunner) IsInstalled() bool {
	return m.Installed
}
