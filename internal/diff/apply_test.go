package diff

import (
	"fmt"
	"testing"

	"brew-sync/internal/manifest"
)

// ---------------------------------------------------------------------------
// unitMockRunner — configurable per-package error mock for unit tests
// ---------------------------------------------------------------------------

// unitMockRunner implements the Runner interface with configurable per-package
// errors. It tracks every call made to Install, Uninstall, and Upgrade.
type unitMockRunner struct {
	// FailOn maps "operation:packageName" to the error to return.
	// Example: "install:badpkg" → fmt.Errorf("install failed")
	FailOn map[string]error

	// Calls records every operation as "operation:packageName".
	Calls []string
}

func newUnitMockRunner() *unitMockRunner {
	return &unitMockRunner{
		FailOn: make(map[string]error),
	}
}

func (m *unitMockRunner) Install(pkg Package) error {
	key := "install:" + pkg.Name
	m.Calls = append(m.Calls, key)
	return m.FailOn[key]
}

func (m *unitMockRunner) Uninstall(pkg Package) error {
	key := "remove:" + pkg.Name
	m.Calls = append(m.Calls, key)
	return m.FailOn[key]
}

func (m *unitMockRunner) Upgrade(pkg Package) error {
	key := "upgrade:" + pkg.Name
	m.Calls = append(m.Calls, key)
	return m.FailOn[key]
}

// ---------------------------------------------------------------------------
// Test 1: Normal apply with mixed operations — all succeed
// Validates: Requirements 4.1, 4.2, 4.3, 4.4
// ---------------------------------------------------------------------------

func TestApplyDiff_NormalApply(t *testing.T) {
	diff := &DiffResult{
		ToInstall: []manifest.PackageEntry{
			{Name: "git", Version: "2.40"},
			{Name: "curl"},
		},
		ToUpgrade: []manifest.PackageEntry{
			{Name: "go", Version: "1.23"},
		},
		ToRemove: []manifest.PackageEntry{
			{Name: "wget"},
		},
		Unchanged: []manifest.PackageEntry{
			{Name: "jq"},
		},
	}

	runner := newUnitMockRunner()
	report, err := ApplyDiff(diff, runner, false)

	// No error when all operations succeed.
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	// Report should not be nil.
	if report == nil {
		t.Fatal("expected non-nil report")
	}

	// Planned should be false for a real apply.
	if report.Planned {
		t.Error("expected Planned=false for normal apply")
	}

	// ErrorCount should be zero.
	if report.ErrorCount != 0 {
		t.Errorf("expected ErrorCount=0, got %d", report.ErrorCount)
	}

	// Should have 4 results: 2 installs + 1 upgrade + 1 remove.
	if len(report.Results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(report.Results))
	}

	// Verify each result recorded correctly.
	expected := []struct {
		op  string
		pkg string
	}{
		{"install", "git"},
		{"install", "curl"},
		{"upgrade", "go"},
		{"remove", "wget"},
	}
	for i, exp := range expected {
		r := report.Results[i]
		if r.Operation != exp.op {
			t.Errorf("result[%d]: expected operation %q, got %q", i, exp.op, r.Operation)
		}
		if r.Package != exp.pkg {
			t.Errorf("result[%d]: expected package %q, got %q", i, exp.pkg, r.Package)
		}
		if r.Err != nil {
			t.Errorf("result[%d]: expected nil error, got %v", i, r.Err)
		}
	}

	// Verify runner received all expected calls.
	if len(runner.Calls) != 4 {
		t.Fatalf("expected 4 runner calls, got %d", len(runner.Calls))
	}
}

// ---------------------------------------------------------------------------
// Test 2: Partial failure — some operations fail, others succeed
// Validates: Requirements 4.2, 4.3, 4.5
// ---------------------------------------------------------------------------

func TestApplyDiff_PartialFailure(t *testing.T) {
	diff := &DiffResult{
		ToInstall: []manifest.PackageEntry{
			{Name: "git"},
			{Name: "badpkg"},
		},
		ToUpgrade: []manifest.PackageEntry{
			{Name: "go", Version: "1.23"},
		},
		ToRemove: []manifest.PackageEntry{
			{Name: "failremove"},
			{Name: "wget"},
		},
	}

	runner := newUnitMockRunner()
	runner.FailOn["install:badpkg"] = fmt.Errorf("install failed: badpkg not found")
	runner.FailOn["remove:failremove"] = fmt.Errorf("remove failed: permission denied")

	report, err := ApplyDiff(diff, runner, false)

	// Should return a non-nil error when there are failures.
	if err == nil {
		t.Fatal("expected non-nil error for partial failure")
	}

	// All 5 operations should be attempted (continue on failure).
	if len(runner.Calls) != 5 {
		t.Fatalf("expected 5 runner calls (all attempted), got %d: %v", len(runner.Calls), runner.Calls)
	}

	// Report should record all 5 results.
	if len(report.Results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(report.Results))
	}

	// ErrorCount should be 2 (badpkg install + failremove remove).
	if report.ErrorCount != 2 {
		t.Errorf("expected ErrorCount=2, got %d", report.ErrorCount)
	}

	// Verify successes and failures are recorded correctly.
	for _, r := range report.Results {
		switch r.Package {
		case "badpkg":
			if r.Err == nil {
				t.Error("expected error for badpkg install")
			}
		case "failremove":
			if r.Err == nil {
				t.Error("expected error for failremove remove")
			}
		case "git", "go", "wget":
			if r.Err != nil {
				t.Errorf("expected nil error for %s %s, got %v", r.Operation, r.Package, r.Err)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Test 3: Dry-run — no runner calls, report has planned counts
// Validates: Requirements 5.1, 5.2, 5.3
// ---------------------------------------------------------------------------

func TestApplyDiff_DryRun(t *testing.T) {
	diff := &DiffResult{
		ToInstall: []manifest.PackageEntry{
			{Name: "git"},
			{Name: "curl"},
			{Name: "jq"},
		},
		ToUpgrade: []manifest.PackageEntry{
			{Name: "go", Version: "1.23"},
			{Name: "node", Version: "20.0"},
		},
		ToRemove: []manifest.PackageEntry{
			{Name: "wget"},
		},
	}

	runner := newUnitMockRunner()
	report, err := ApplyDiff(diff, runner, true)

	// Dry-run should not return an error.
	if err != nil {
		t.Fatalf("expected nil error for dry-run, got: %v", err)
	}

	// Report should have Planned=true.
	if !report.Planned {
		t.Error("expected Planned=true for dry-run")
	}

	// Counts should match the diff sizes.
	if report.InstallCount != 3 {
		t.Errorf("expected InstallCount=3, got %d", report.InstallCount)
	}
	if report.UpgradeCount != 2 {
		t.Errorf("expected UpgradeCount=2, got %d", report.UpgradeCount)
	}
	if report.RemoveCount != 1 {
		t.Errorf("expected RemoveCount=1, got %d", report.RemoveCount)
	}

	// Zero runner calls in dry-run mode.
	if len(runner.Calls) != 0 {
		t.Errorf("expected 0 runner calls in dry-run, got %d: %v", len(runner.Calls), runner.Calls)
	}

	// No results should be recorded in dry-run.
	if len(report.Results) != 0 {
		t.Errorf("expected 0 results in dry-run, got %d", len(report.Results))
	}
}

// ---------------------------------------------------------------------------
// Test 4: Empty diff — nothing to do
// Validates: Requirements 4.1, 4.2
// ---------------------------------------------------------------------------

func TestApplyDiff_EmptyDiff(t *testing.T) {
	diff := &DiffResult{}

	runner := newUnitMockRunner()
	report, err := ApplyDiff(diff, runner, false)

	// No error for empty diff.
	if err != nil {
		t.Fatalf("expected nil error for empty diff, got: %v", err)
	}

	// Report should have no results.
	if len(report.Results) != 0 {
		t.Errorf("expected 0 results, got %d", len(report.Results))
	}

	// ErrorCount should be zero.
	if report.ErrorCount != 0 {
		t.Errorf("expected ErrorCount=0, got %d", report.ErrorCount)
	}

	// Zero runner calls.
	if len(runner.Calls) != 0 {
		t.Errorf("expected 0 runner calls, got %d", len(runner.Calls))
	}
}

// ---------------------------------------------------------------------------
// Test 5: All operations fail — error count matches total
// Validates: Requirements 4.3, 4.5
// ---------------------------------------------------------------------------

func TestApplyDiff_AllFail(t *testing.T) {
	diff := &DiffResult{
		ToInstall: []manifest.PackageEntry{
			{Name: "pkg1"},
			{Name: "pkg2"},
		},
		ToUpgrade: []manifest.PackageEntry{
			{Name: "pkg3", Version: "2.0"},
		},
		ToRemove: []manifest.PackageEntry{
			{Name: "pkg4"},
		},
	}

	runner := newUnitMockRunner()
	runner.FailOn["install:pkg1"] = fmt.Errorf("fail")
	runner.FailOn["install:pkg2"] = fmt.Errorf("fail")
	runner.FailOn["upgrade:pkg3"] = fmt.Errorf("fail")
	runner.FailOn["remove:pkg4"] = fmt.Errorf("fail")

	report, err := ApplyDiff(diff, runner, false)

	// Should return a non-nil error.
	if err == nil {
		t.Fatal("expected non-nil error when all operations fail")
	}

	// All 4 operations should still be attempted.
	if len(runner.Calls) != 4 {
		t.Fatalf("expected 4 runner calls, got %d", len(runner.Calls))
	}

	// ErrorCount should match total operations.
	totalOps := len(diff.ToInstall) + len(diff.ToUpgrade) + len(diff.ToRemove)
	if report.ErrorCount != totalOps {
		t.Errorf("expected ErrorCount=%d, got %d", totalOps, report.ErrorCount)
	}

	// Every result should have a non-nil error.
	for i, r := range report.Results {
		if r.Err == nil {
			t.Errorf("result[%d] (%s %s): expected non-nil error", i, r.Operation, r.Package)
		}
	}
}
