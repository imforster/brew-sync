# Implementation Plan: brew-sync

## Overview

Implement a Go CLI tool that wraps Homebrew to synchronize installed packages across multiple machines. The tool uses a declarative TOML manifest, computes diffs between desired and actual state, and applies changes via the `brew` CLI. Synchronization is pull-based using configurable backends (Git or file). Built with `cobra` for CLI, `BurntSushi/toml` for manifest serialization, and `pgregory.net/rapid` for property-based testing.

## Tasks

- [x] 1. Set up project structure and core data models
  - [x] 1.1 Initialize Go module and directory structure
    - Initialize Go module (`go mod init`)
    - Create directory layout: `cmd/`, `internal/manifest/`, `internal/brew/`, `internal/diff/`, `internal/sync/`, `internal/config/`
    - Add dependencies: `github.com/spf13/cobra`, `github.com/BurntSushi/toml`, `pgregory.net/rapid`
    - _Requirements: 11.1, 11.2_
  - [x] 1.2 Define core data model types
    - Create `internal/manifest/types.go` with `Manifest`, `ManifestMetadata`, `PackageEntry` structs with TOML tags
    - Create `internal/diff/types.go` with `DiffResult`, `LocalState`, `Package` structs
    - Create `internal/config/config.go` with `Config`, `GitConfig`, `FileConfig` structs
    - Create `internal/brew/types.go` with `ApplyReport` struct and `RecordResult`/`HasErrors` methods
    - _Requirements: 8.1, 2.1_

- [x] 2. Implement Manifest Manager
  - [x] 2.1 Implement manifest Load and Save
    - Create `internal/manifest/manager.go` with `ManifestManager` struct
    - Implement `Load(path string) (*Manifest, error)` to read and deserialize TOML
    - Implement `Save(path string, m *Manifest) error` to serialize and write TOML
    - _Requirements: 8.1, 8.2_
  - [x] 2.2 Write property test for manifest round-trip
    - **Property 5: Manifest round-trip** — `Load(Save(m)) == m` for any valid manifest
    - **Validates: Requirements 8.1, 8.2**
  - [x] 2.3 Implement manifest Validate
    - Implement `Validate(m *Manifest) error` with multi-error collection
    - Check: unsupported version, duplicate names in formulae/casks, empty names, mutual exclusivity of `only_on`/`except_on`, tap format `owner/repo`
    - _Requirements: 8.3, 8.4, 8.5, 8.6, 8.7, 8.8_
  - [x] 2.4 Write property test for validation completeness
    - **Property 6: Validation completeness** — A manifest with duplicate names, empty names, both `only_on` and `except_on` set, or invalid tap format is rejected by `Validate`
    - **Validates: Requirements 8.3, 8.4, 8.5, 8.6, 8.7**
  - [x] 2.5 Implement BuildFromLocal
    - Implement `BuildFromLocal(formulae []Package, casks []Package, taps []string) *Manifest`
    - Set version to 1, sort formulae/casks by name, sort taps, set `updated_at` to current time, leave machine filters empty
    - _Requirements: 1.2, 1.3_
  - [x] 2.6 Write unit tests for Manifest Manager
    - Test Load/Save with valid manifests, test Validate with each invalid case, test BuildFromLocal with empty and populated inputs
    - _Requirements: 8.1, 8.2, 8.3, 8.4, 8.5, 8.6, 8.7, 8.8, 1.2, 1.3_

- [x] 3. Implement Brew Runner
  - [x] 3.1 Implement BrewRunner interface and real implementation
    - Create `internal/brew/runner.go` with `BrewRunner` interface: `ListFormulae`, `ListCasks`, `ListTaps`, `Install`, `Uninstall`, `Upgrade`, `Update`, `IsInstalled`
    - Implement `RealBrewRunner` that executes `brew` commands via `os/exec.Command` (no shell string construction)
    - Parse `brew list --formula --versions` and `brew list --cask --versions` output
    - Implement `IsInstalled()` to check if `brew` binary exists in PATH
    - _Requirements: 9.1, 9.2, 13.1_
  - [x] 3.2 Implement mock BrewRunner for testing
    - Create `internal/brew/mock_runner.go` with `MockBrewRunner` that records calls and returns configurable results
    - Track call counts for Install/Uninstall/Upgrade to verify dry-run safety
    - _Requirements: 5.2_
  - [x] 3.3 Write unit tests for BrewRunner output parsing
    - Test parsing of `brew list` output with various formats
    - Test `IsInstalled` with missing brew binary
    - _Requirements: 9.1, 9.2, 13.1_

- [x] 4. Checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 5. Implement Diff Engine
  - [x] 5.1 Implement FilterForMachine
    - Create `internal/diff/filter.go` with `FilterForMachine(entries []PackageEntry, machineTag string) []PackageEntry`
    - Include entry if `only_on` is set and machineTag is in the list
    - Exclude entry if `except_on` is set and machineTag is in the list
    - Always include entry if neither filter is set
    - Do not modify the original slice
    - _Requirements: 3.1, 3.2, 3.3_
  - [x] 5.2 Write property test for machine filter correctness
    - **Property 3: Machine filter correctness** — Packages with `only_on = [X]` where `machineTag ≠ X` are excluded; packages with `except_on = [X]` where `machineTag = X` are excluded
    - **Property 8: Filter mutual exclusivity** — `FilterForMachine` never includes a package where `machineTag` is in `ExceptOn`, and never excludes a package where `machineTag` is in `OnlyOn`
    - **Validates: Requirements 3.1, 3.2, 3.3**
  - [x] 5.3 Implement ComputeDiff
    - Create `internal/diff/engine.go` with `ComputeDiff(manifest *Manifest, local *LocalState, machineTag string) *DiffResult`
    - Filter manifest entries for current machine, build local lookup maps, classify each package into exactly one of ToInstall/ToRemove/ToUpgrade/Unchanged
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7_
  - [x] 5.4 Write property test for diff completeness and soundness
    - **Property 1: Diff completeness** — Every package in manifest ∪ local appears in exactly one of {ToInstall, ToRemove, ToUpgrade, Unchanged}
    - **Property 2: Diff soundness** — ToInstall contains only manifest-only packages; ToRemove contains only local-only packages
    - **Validates: Requirements 2.2, 2.3, 2.4, 2.5, 2.6**
  - [x] 5.5 Write unit tests for Diff Engine
    - Test with empty sets, identical sets, disjoint sets, partial overlaps, version differences, and machine-filtered entries
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7, 3.1, 3.2, 3.3_

- [x] 6. Implement Apply Logic
  - [x] 6.1 Implement ApplyDiff
    - Create `internal/diff/apply.go` with `ApplyDiff(diff *DiffResult, runner BrewRunner, dryRun bool) (*ApplyReport, error)`
    - In dry-run mode: populate report with planned counts, make zero calls to runner
    - In normal mode: install, upgrade, remove packages; continue on individual failure; collect results into report
    - Return aggregate error if any operations failed
    - _Requirements: 4.1, 4.2, 4.3, 4.4, 4.5, 5.1, 5.2, 5.3_
  - [x] 6.2 Write property test for dry-run safety
    - **Property 7: Dry-run safety** — When `dryRun = true`, `ApplyDiff` produces a report but makes zero calls to `runner.Install`, `runner.Uninstall`, or `runner.Upgrade`
    - **Validates: Requirements 5.1, 5.2**
  - [x] 6.3 Write property test for idempotency
    - **Property 4: Idempotency** — Applying a diff and recomputing the diff yields an empty diff (no installs, no removes, no upgrades)
    - **Validates: Requirements 4.1, 2.2**
  - [x] 6.4 Write unit tests for ApplyDiff
    - Test normal apply with mixed results, test partial failure handling, test dry-run output
    - _Requirements: 4.1, 4.2, 4.3, 4.4, 4.5, 5.1, 5.2, 5.3_

- [x] 7. Checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 8. Implement Sync Backends
  - [x] 8.1 Implement SyncBackend interface and Git backend
    - Create `internal/sync/backend.go` with `SyncBackend` interface: `Pull(dest string) error`, `Push(src string) error`, `Name() string`
    - Implement `GitBackend` that clones/pulls from a configured repo URL and branch, and pushes manifest commits
    - Handle authentication errors and connectivity failures with descriptive messages
    - _Requirements: 12.1, 12.3, 12.5_
  - [x] 8.2 Implement File sync backend
    - Implement `FileBackend` that copies the manifest to/from a configured shared filesystem path
    - Handle file-not-found and permission errors with descriptive messages
    - _Requirements: 12.2, 12.4, 12.5_
  - [x] 8.3 Implement backend factory
    - Create `NewSyncBackend(config *Config) (SyncBackend, error)` factory function that returns the appropriate backend based on config
    - _Requirements: 12.1, 12.2_
  - [x] 8.4 Write unit tests for Sync Backends
    - Test Git backend with a local bare repository
    - Test File backend with temp directories
    - Test factory function with valid and invalid configs
    - _Requirements: 12.1, 12.2, 12.3, 12.4, 12.5_

- [ ] 9. Implement CLI Commands
  - [x] 9.1 Implement root command and global flags
    - Create `cmd/root.go` with cobra root command
    - Register global flags: `--config`, `--verbose`, `--dry-run`
    - Add brew prerequisite check (exit with message if brew not found)
    - Add usage help for unknown subcommands/invalid flags
    - _Requirements: 11.1, 11.2, 11.3, 9.1, 9.2_
  - [x] 9.2 Implement `init` command
    - Create `cmd/init.go` that queries local brew state via BrewRunner and builds/saves manifest via ManifestManager
    - _Requirements: 1.1, 1.2, 1.3, 1.4_
  - [x] 9.3 Implement `status` command
    - Create `cmd/status.go` that loads manifest, gets local state, computes diff, and prints human-readable summary
    - Handle manifest-not-found with helpful error message
    - _Requirements: 2.1, 10.1_
  - [x] 9.4 Implement `apply` command
    - Create `cmd/apply.go` that loads manifest, computes diff, applies diff (respecting `--dry-run`), and prints report
    - Handle manifest-not-found with helpful error message
    - Exit with non-zero code on failures
    - _Requirements: 4.1, 4.2, 4.3, 4.4, 4.5, 5.1, 5.2, 5.3, 10.1_
  - [x] 9.5 Implement `push` command
    - Create `cmd/push.go` that snapshots local state, builds manifest, saves it, and pushes via sync backend
    - _Requirements: 6.1, 6.2, 6.3, 6.4_
  - [x] 9.6 Implement `pull` command
    - Create `cmd/pull.go` that fetches manifest from remote via sync backend and writes to local path
    - Handle unreachable backend with descriptive error
    - _Requirements: 7.1, 7.2, 7.3_
  - [x] 9.7 Implement `main.go` entry point
    - Create `main.go` that executes the root command
    - _Requirements: 11.1_

- [x] 10. Checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 11. Integration wiring and final verification
  - [x] 11.1 Wire config loading into CLI commands
    - Load config from `--config` flag path (default `~/.config/brew-sync/config.toml`)
    - Initialize BrewRunner, ManifestManager, SyncBackend from config
    - Pass machine tag from config into diff computations
    - _Requirements: 12.1, 12.2, 12.3, 12.4_
  - [x] 11.2 Add verbose output support
    - When `--verbose` is set, print detailed operation logs during apply, push, and pull
    - _Requirements: 11.2_
  - [x] 11.3 Write integration tests for full workflows
    - Test init → push → pull → status → apply workflow using MockBrewRunner
    - Test apply with `--dry-run` end-to-end
    - Test error paths: missing brew, missing manifest, unreachable backend
    - _Requirements: 1.1, 2.1, 4.1, 5.1, 6.1, 7.1, 9.1, 10.1_

- [x] 12. Final checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

## Notes

- Tasks marked with `*` are optional and can be skipped for faster MVP
- Each task references specific requirements for traceability
- Checkpoints ensure incremental validation
- Property tests validate universal correctness properties from the design document using `pgregory.net/rapid`
- Unit tests validate specific examples and edge cases using Go's standard `testing` package
- The MockBrewRunner is essential for testing without requiring actual Homebrew installation
- All `brew` commands use `exec.Command` argument passing (no shell strings) per security requirement 13.1
