# Requirements Document

## Introduction

brew-sync is a Go CLI tool that wraps Homebrew to synchronize installed packages (formulae and casks) across multiple machines. Users maintain a declarative TOML manifest file describing the desired set of packages. The tool detects drift between the manifest and the local Homebrew installation, then applies changes to converge the local state to the declared state. Synchronization follows a pull-based model where each machine independently pulls a shared manifest from a configurable sync backend (Git repository or shared file) and reconciles locally.

## Glossary

- **CLI**: The brew-sync command-line interface that parses arguments and dispatches to subcommand handlers
- **Manifest**: A TOML file (brew-sync.toml) declaring the desired set of packages, taps, and metadata
- **Manifest_Manager**: The component responsible for reading, writing, validating, and building manifest files
- **Brew_Runner**: The component that wraps the `brew` CLI to query and mutate the local Homebrew installation
- **Diff_Engine**: The component that compares the manifest against locally installed packages and produces a diff result
- **Sync_Backend**: The component that handles pushing and pulling the manifest to and from a remote location (Git or file)
- **DiffResult**: A data structure containing four disjoint sets: ToInstall, ToRemove, ToUpgrade, and Unchanged
- **PackageEntry**: A manifest entry describing a package with name, optional version, and optional machine filters
- **Machine_Tag**: A string identifier for the current machine, used to evaluate per-package inclusion filters
- **Formulae**: Homebrew command-line packages
- **Casks**: Homebrew GUI application packages
- **Tap**: A third-party Homebrew repository in `owner/repo` format

## Requirements

### Requirement 1: Initialize a Manifest from Local State

**User Story:** As a user, I want to generate a manifest from my current Homebrew installation, so that I can capture my existing packages as the starting point for synchronization.

#### Acceptance Criteria

1. WHEN a user runs the `init` command, THE Brew_Runner SHALL query the locally installed formulae, casks, and taps
2. WHEN the local packages are retrieved, THE Manifest_Manager SHALL build a manifest with version set to 1, formulae sorted by name, casks sorted by name, and taps sorted alphabetically
3. WHEN a manifest is built from local state, THE Manifest_Manager SHALL set the metadata `updated_at` field to the current time and leave `only_on` and `except_on` filters empty on all entries
4. WHEN the manifest is built, THE Manifest_Manager SHALL write the manifest to the configured file path in TOML format

### Requirement 2: Compute Drift Between Manifest and Local State

**User Story:** As a user, I want to see what differs between my manifest and my local Homebrew installation, so that I can understand what changes would be applied before synchronizing.

#### Acceptance Criteria

1. WHEN a user runs the `status` command, THE Diff_Engine SHALL compare the manifest against the locally installed packages and produce a DiffResult
2. THE Diff_Engine SHALL classify every package in the union of manifest entries and local packages into exactly one of: ToInstall, ToRemove, ToUpgrade, or Unchanged
3. WHEN a package is present in the manifest but not installed locally, THE Diff_Engine SHALL classify the package as ToInstall
4. WHEN a package is installed locally but not present in the manifest, THE Diff_Engine SHALL classify the package as ToRemove
5. WHEN a package is present in both the manifest and locally with a different version specified in the manifest, THE Diff_Engine SHALL classify the package as ToUpgrade
6. WHEN a package is present in both the manifest and locally with no version difference, THE Diff_Engine SHALL classify the package as Unchanged
7. WHEN computing the diff, THE Diff_Engine SHALL apply machine-specific filters before classification so that filtered-out packages are excluded from ToInstall

### Requirement 3: Filter Packages by Machine Tag

**User Story:** As a user, I want to include or exclude specific packages per machine, so that I can maintain a single manifest for machines with different needs.

#### Acceptance Criteria

1. WHEN a PackageEntry has an `only_on` list set, THE Diff_Engine SHALL include the entry only if the current Machine_Tag is in the `only_on` list
2. WHEN a PackageEntry has an `except_on` list set, THE Diff_Engine SHALL include the entry only if the current Machine_Tag is NOT in the `except_on` list
3. WHEN a PackageEntry has neither `only_on` nor `except_on` set, THE Diff_Engine SHALL always include the entry
4. THE Manifest_Manager SHALL reject any PackageEntry that has both `only_on` and `except_on` set during validation

### Requirement 4: Apply Diff to Local Homebrew Installation

**User Story:** As a user, I want to apply the computed diff to my local machine, so that my Homebrew installation converges to the declared manifest state.

#### Acceptance Criteria

1. WHEN a user runs the `apply` command, THE Brew_Runner SHALL install all packages in the ToInstall set, upgrade all packages in the ToUpgrade set, and uninstall all packages in the ToRemove set
2. WHEN applying changes, THE CLI SHALL produce an ApplyReport containing the success or failure status of each individual operation
3. IF a single package operation fails during apply, THEN THE CLI SHALL continue processing the remaining packages and record the failure in the ApplyReport
4. WHEN all operations complete, THE CLI SHALL display a summary report to the user
5. IF any operations failed, THEN THE CLI SHALL return a non-zero exit code and report the number of errors

### Requirement 5: Dry-Run Mode

**User Story:** As a user, I want to preview what changes would be applied without actually modifying my system, so that I can verify the sync plan before committing to it.

#### Acceptance Criteria

1. WHEN the `--dry-run` flag is set, THE CLI SHALL compute and display the planned actions without executing any install, uninstall, or upgrade operations
2. WHILE the `--dry-run` flag is active, THE Brew_Runner SHALL receive zero calls to Install, Uninstall, or Upgrade
3. WHEN a dry-run completes, THE CLI SHALL display the count of packages that would be installed, removed, and upgraded

### Requirement 6: Push Manifest to Remote

**User Story:** As a user, I want to push my local manifest to a remote location, so that other machines can pull and synchronize from it.

#### Acceptance Criteria

1. WHEN a user runs the `push` command, THE Brew_Runner SHALL snapshot the current locally installed formulae and casks
2. WHEN the local snapshot is taken, THE Manifest_Manager SHALL build and save a manifest from the local state
3. WHEN the manifest is saved, THE Sync_Backend SHALL push the manifest to the configured remote location
4. WHEN the push succeeds, THE CLI SHALL display a confirmation message to the user

### Requirement 7: Pull Manifest from Remote

**User Story:** As a user, I want to pull the shared manifest from a remote location, so that I can synchronize my machine to the latest declared state.

#### Acceptance Criteria

1. WHEN a user runs the `pull` command, THE Sync_Backend SHALL fetch the manifest from the configured remote location
2. WHEN the remote manifest is fetched, THE Manifest_Manager SHALL write the manifest to the local file path
3. IF the remote location is unreachable, THEN THE Sync_Backend SHALL return an error with backend-specific details

### Requirement 8: Manifest Serialization and Validation

**User Story:** As a user, I want the manifest file to be validated and reliably serialized, so that I can trust the manifest is correct and no data is lost during read/write cycles.

#### Acceptance Criteria

1. THE Manifest_Manager SHALL serialize and deserialize manifests in TOML format preserving all fields including version, metadata, formulae, casks, and taps
2. WHEN loading a manifest, THE Manifest_Manager SHALL produce an object that, when saved and loaded again, is equivalent to the original (round-trip preservation)
3. WHEN validating a manifest, THE Manifest_Manager SHALL reject manifests with an unsupported version number
4. WHEN validating a manifest, THE Manifest_Manager SHALL reject manifests containing duplicate package names within the formulae section or within the casks section
5. WHEN validating a manifest, THE Manifest_Manager SHALL reject manifests containing a PackageEntry with an empty name
6. WHEN validating a manifest, THE Manifest_Manager SHALL reject manifests containing a PackageEntry with both `only_on` and `except_on` set
7. WHEN validating a manifest, THE Manifest_Manager SHALL reject manifests containing a tap entry that does not match the `owner/repo` format
8. WHEN validation fails, THE Manifest_Manager SHALL return all validation errors collected together so the user can fix them in one pass

### Requirement 9: Brew CLI Prerequisite Check

**User Story:** As a user, I want clear feedback if Homebrew is not installed, so that I understand why the tool cannot operate.

#### Acceptance Criteria

1. WHEN the `brew` binary is not found in PATH, THE CLI SHALL display the message "brew not found. Please install Homebrew first" with a link to the Homebrew website
2. WHEN the `brew` binary is not found, THE CLI SHALL exit with a non-zero status code without attempting any package operations

### Requirement 10: Manifest Not Found Handling

**User Story:** As a user, I want helpful guidance when no manifest exists, so that I know how to create one.

#### Acceptance Criteria

1. IF the manifest file does not exist when running `status` or `apply`, THEN THE CLI SHALL return an error suggesting the user run `brew-sync init` or `brew-sync pull`

### Requirement 11: CLI Command Structure

**User Story:** As a user, I want a consistent and predictable command-line interface, so that I can use the tool efficiently.

#### Acceptance Criteria

1. THE CLI SHALL provide the subcommands: init, status, push, pull, and apply
2. THE CLI SHALL accept global flags: `--config` for configuration file path, `--verbose` for detailed output, and `--dry-run` for preview mode
3. WHEN an unknown subcommand or invalid flag is provided, THE CLI SHALL display a usage help message and exit with a non-zero status code

### Requirement 12: Sync Backend Configuration

**User Story:** As a user, I want to configure how my manifest is synchronized, so that I can use the sync mechanism that fits my workflow.

#### Acceptance Criteria

1. THE CLI SHALL support a `git` sync backend that pushes and pulls the manifest via a Git repository
2. THE CLI SHALL support a `file` sync backend that copies the manifest to and from a shared filesystem path
3. WHEN the sync backend is configured as `git`, THE Sync_Backend SHALL use the configured repository URL and branch
4. WHEN the sync backend is configured as `file`, THE Sync_Backend SHALL use the configured remote file path
5. IF the configured sync backend is unreachable during push or pull, THEN THE Sync_Backend SHALL return an error with backend-specific diagnostic details

### Requirement 13: Security and Command Execution Safety

**User Story:** As a user, I want the tool to execute brew commands safely, so that malicious manifest content cannot compromise my system through command injection.

#### Acceptance Criteria

1. THE Brew_Runner SHALL pass package names as arguments to `exec.Command` rather than constructing shell command strings
2. THE CLI SHALL support a `status` command and `--dry-run` flag so users can review diffs before applying changes from untrusted manifests
