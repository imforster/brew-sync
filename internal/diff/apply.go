package diff

import (
	"fmt"
)

// Runner abstracts the Homebrew CLI operations needed by ApplyDiff.
// This interface is satisfied by brew.RealBrewRunner and brew.MockBrewRunner.
type Runner interface {
	Install(pkg Package) error
	Uninstall(pkg Package) error
	Upgrade(pkg Package) error
}

// ApplyReport tracks the results of applying a diff to the local Homebrew installation.
type ApplyReport struct {
	Planned            bool
	InstallCount       int
	RemoveCount        int
	UpgradeCount       int
	SkippedRemoveCount int
	Results            []OperationResult
	ErrorCount         int
}

// OperationResult records the outcome of a single brew operation.
type OperationResult struct {
	Operation string
	Package   string
	Err       error
}

// RecordResult appends an operation result to the report and increments the error count if err is non-nil.
func (r *ApplyReport) RecordResult(operation, pkgName string, err error) {
	r.Results = append(r.Results, OperationResult{
		Operation: operation,
		Package:   pkgName,
		Err:       err,
	})
	if err != nil {
		r.ErrorCount++
	}
}

// HasErrors returns true if any operations recorded in the report failed.
func (r *ApplyReport) HasErrors() bool {
	return r.ErrorCount > 0
}

// ApplyOptions controls the behavior of ApplyDiff.
type ApplyOptions struct {
	DryRun     bool
	SkipRemove bool
}

// ApplyDiff executes the diff against the local Homebrew installation.
//
// Algorithm:
//  1. If dry-run, populate report with planned counts and return (zero mutations)
//  2. Install missing packages from ToInstall
//  3. Upgrade outdated packages from ToUpgrade
//  4. Remove extra packages from ToRemove (unless SkipRemove is true)
//  5. Collect results into report
//
// Partial failure strategy: continue on individual package failure,
// record error in report, return aggregate error at end.
func ApplyDiff(diff *DiffResult, runner Runner, dryRun bool, opts ...ApplyOptions) (*ApplyReport, error) {
	report := &ApplyReport{}

	skipRemove := false
	if len(opts) > 0 {
		skipRemove = opts[0].SkipRemove
	}

	if dryRun {
		report.Planned = true
		report.InstallCount = len(diff.ToInstall)
		if skipRemove {
			report.RemoveCount = 0
			report.SkippedRemoveCount = len(diff.ToRemove)
		} else {
			report.RemoveCount = len(diff.ToRemove)
		}
		report.UpgradeCount = len(diff.ToUpgrade)
		return report, nil
	}

	// Install missing packages
	for _, pkg := range diff.ToInstall {
		err := runner.Install(Package{Name: pkg.Name, Version: pkg.Version})
		report.RecordResult("install", pkg.Name, err)
	}

	// Upgrade outdated packages
	for _, pkg := range diff.ToUpgrade {
		err := runner.Upgrade(Package{Name: pkg.Name, Version: pkg.Version})
		report.RecordResult("upgrade", pkg.Name, err)
	}

	// Remove extra packages (unless SkipRemove is set)
	if skipRemove {
		report.SkippedRemoveCount = len(diff.ToRemove)
	} else {
		for _, pkg := range diff.ToRemove {
			err := runner.Uninstall(Package{Name: pkg.Name})
			report.RecordResult("remove", pkg.Name, err)
		}
	}

	if report.HasErrors() {
		return report, fmt.Errorf("apply completed with %d errors", report.ErrorCount)
	}
	return report, nil
}
