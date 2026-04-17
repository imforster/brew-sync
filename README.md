# brew-sync User Guide

brew-sync is a CLI tool that synchronizes your Homebrew packages across multiple machines using a declarative TOML manifest. You describe the packages you want, and brew-sync handles installing, upgrading, and removing packages to match.

## Table of Contents

- [Installation](#installation)
- [Quick Start](#quick-start)
- [Commands](#commands)
- [Configuration](#configuration)
- [The Manifest File](#the-manifest-file)
- [Machine-Specific Packages](#machine-specific-packages)
- [Sync Backends](#sync-backends)
- [Workflows](#workflows)
- [Troubleshooting](#troubleshooting)

## Installation

### Prerequisites

- macOS with [Homebrew](https://brew.sh) installed
- Go 1.25+ (for building from source)

### Build from Source

```bash
# Clone the repository
git clone <repo-url>
cd brew-sync

# Build
make build

# The binary is at build/brew-sync
./build/brew-sync --help

# Or install to your GOPATH
make install
```

## Quick Start

### 1. Capture your current packages

```bash
brew-sync init
```

This scans your Homebrew installation and creates a `brew-sync.toml` manifest with all your currently installed formulae, casks, and taps.

```
Manifest written to brew-sync.toml (47 formulae, 12 casks, 3 taps)
```

### 2. Check what would change

```bash
brew-sync status
```

Compares the manifest against your local installation and shows the drift:

```
3 packages to install
1 packages to remove
2 packages to upgrade
44 packages unchanged
```

### 3. Apply the manifest

```bash
brew-sync apply
```

Installs missing packages and upgrades outdated ones. Local-only packages are kept by default:

```
  ✓ install ripgrep
  ✓ install fd
  ✓ install bat
  ✓ upgrade go
  ✓ upgrade node

5 succeeded, 0 failed out of 5 operations
2 local-only packages kept (use --remove to uninstall)
```

### 4. Preview before applying

```bash
brew-sync apply --dry-run
```

Shows what would happen without making any changes:

```
Would install 3, upgrade 2 packages
2 local-only packages kept (use --remove to uninstall)
```

### 5. Reconcile local-only packages

```bash
brew-sync reconcile
```

Walks through packages installed locally but not in the manifest. For each one, choose to add it to the manifest (for all machines or just this one) or skip it.

## Commands

### `brew-sync init`

Generates a manifest from your current Homebrew installation.

```bash
brew-sync init
```

This creates `brew-sync.toml` (or the path set in your config) containing all locally installed formulae, casks, and taps. Packages are sorted alphabetically. No machine filters are applied — the snapshot reflects exactly what is installed.

### `brew-sync status`

Shows the difference between the manifest and your local packages.

```bash
brew-sync status

# With detailed package list
brew-sync status --verbose
```

Verbose output shows individual packages:

```
3 packages to install
1 packages to remove
0 packages to upgrade
44 packages unchanged

To install:
  + ripgrep
  + fd
  + bat

To remove:
  - oldtool
```

### `brew-sync apply`

Applies the manifest to your local machine. Installs missing packages and upgrades outdated ones.

By default, packages installed locally but not in the manifest are kept. Use `--remove` to also uninstall them.

```bash
# Apply changes (installs and upgrades only)
brew-sync apply

# Also remove packages not in the manifest
brew-sync apply --remove

# Preview only (no changes made)
brew-sync apply --dry-run

# With detailed logging
brew-sync apply --verbose
```

If some packages fail, brew-sync continues with the rest and reports failures at the end. The exit code is non-zero when any operation fails.

### `brew-sync reconcile`

Interactively walks through packages installed locally but not in the manifest. For each one, you choose:

- **a** — Add to manifest for all machines
- **m** — Add to manifest only for this machine (`only_on`)
- **s** — Skip (leave installed, don't add to manifest)

```bash
brew-sync reconcile
```

```
Found 3 local-only packages not in the manifest.
Machine tag: work-macbook

  wget (formula)
    [a] Add to manifest (all machines)
    [m] Add to manifest (only on work-macbook)
    [s] Skip
    Choice: a
    → Added wget for all machines

  docker-desktop (cask)
    [a] Add to manifest (all machines)
    [m] Add to manifest (only on work-macbook)
    [s] Skip
    Choice: m
    → Added docker-desktop (only_on: work-macbook)

  htop (formula)
    [a] Add to manifest (all machines)
    [m] Add to manifest (only on work-macbook)
    [s] Skip
    Choice: s
    → Skipped

Manifest updated: 1 added for all machines, 1 added for this machine only, 1 skipped
```

This is useful after pulling a manifest from another machine to incorporate packages unique to this one.

### `brew-sync push`

Snapshots your local Homebrew state, builds a manifest, and pushes it to the configured remote backend.

```bash
brew-sync push
```

```
Manifest saved to brew-sync.toml (47 formulae, 12 casks, 3 taps)
Manifest pushed successfully via git backend.
```

If no sync backend is configured, the manifest is saved locally only.

### `brew-sync pull`

Fetches the shared manifest from the configured remote backend.

```bash
brew-sync pull
```

```
Manifest pulled successfully via file backend and saved to brew-sync.toml.
```

After pulling, run `status` or `apply` to see and apply changes.

## Configuration

brew-sync looks for a config file at `~/.config/brew-sync/config.toml` by default. Override with `--config`:

```bash
brew-sync --config /path/to/config.toml status
```

To get started, copy the example config and edit it:

```bash
mkdir -p ~/.config/brew-sync
cp config.toml.example ~/.config/brew-sync/config.toml
```

The example config includes step-by-step instructions for setting up GitHub as your sync backend.

### Config File Format

```toml
# Path to the manifest file (default: brew-sync.toml)
manifest_path = "brew-sync.toml"

# Identifier for this machine, used for per-machine package filtering
machine_tag = "work-macbook"

# Sync backend: "git" or "file"
sync_backend = "git"

# Git backend settings
[git]
repo_url = "git@github.com:youruser/brew-sync-manifest.git"
branch = "main"

# File backend settings (alternative to git)
[file]
remote_path = "/Volumes/shared/brew-sync.toml"
```

### Global Flags

| Flag | Description |
|---|---|
| `--config <path>` | Path to config file (default: `~/.config/brew-sync/config.toml`) |
| `--verbose` | Print detailed operation logs |
| `--dry-run` | Preview changes without applying them |
| `--remove` | Also uninstall packages not in the manifest (apply only) |

## The Manifest File

The manifest (`brew-sync.toml`) is a TOML file that declares your desired packages.

### Example

```toml
version = 1

[metadata]
updated_at = "2025-01-15T10:30:00Z"
updated_by = "alice"
machine = "alice-macbook"

taps = ["homebrew/cask-fonts", "hashicorp/tap"]

[[formulae]]
name = "git"

[[formulae]]
name = "go"
version = "1.23"

[[formulae]]
name = "docker"
only_on = ["work-laptop"]

[[casks]]
name = "firefox"

[[casks]]
name = "slack"
except_on = ["home-desktop"]
```

### Fields

- `version` — Schema version (must be `1`)
- `metadata.updated_at` — Timestamp of last update
- `metadata.updated_by` — Who last updated the manifest
- `metadata.machine` — Machine that generated the manifest
- `taps` — Third-party Homebrew repositories (format: `owner/repo`)
- `formulae` — Command-line packages
- `casks` — GUI application packages

Each package entry supports:

| Field | Required | Description |
|---|---|---|
| `name` | Yes | Package name |
| `version` | No | Pin to a specific version |
| `only_on` | No | Only install on these machines |
| `except_on` | No | Skip installation on these machines |

### Validation Rules

brew-sync validates the manifest and reports all errors at once:

- Version must be `1`
- Package names cannot be empty
- No duplicate names within formulae or within casks
- `only_on` and `except_on` cannot both be set on the same package
- Taps must use `owner/repo` format

## Machine-Specific Packages

Use `only_on` and `except_on` to control which packages install on which machines. The machine is identified by the `machine_tag` in your config file.

### Install only on specific machines

```toml
[[formulae]]
name = "docker"
only_on = ["work-laptop", "ci-server"]
```

Docker will only be installed on machines with `machine_tag` set to `work-laptop` or `ci-server`.

### Skip on specific machines

```toml
[[casks]]
name = "slack"
except_on = ["home-desktop"]
```

Slack will be installed everywhere except on the machine tagged `home-desktop`.

### No filter (default)

```toml
[[formulae]]
name = "git"
```

Packages without filters are installed on all machines.

## Sync Backends

brew-sync supports two backends for sharing the manifest between machines.

### Git Backend

Stores the manifest in a Git repository. Good for version history and collaboration.

```toml
sync_backend = "git"

[git]
repo_url = "git@github.com:youruser/brew-sync-manifest.git"
branch = "main"
```

Git authentication uses your existing SSH/git configuration — brew-sync does not handle credentials directly.

### File Backend

Copies the manifest to/from a shared filesystem path. Good for local networks, NAS, or cloud-synced folders (Dropbox, iCloud Drive, etc.).

```toml
sync_backend = "file"

[file]
remote_path = "/Users/alice/Library/Mobile Documents/com~apple~CloudDocs/brew-sync.toml"
```

## Workflows

### Setting up a new machine

On your existing machine:

```bash
# Capture current packages and push
brew-sync init
brew-sync push
```

On the new machine:

```bash
# Create a config file with the same sync backend
mkdir -p ~/.config/brew-sync
cat > ~/.config/brew-sync/config.toml << 'EOF'
manifest_path = "brew-sync.toml"
machine_tag = "new-macbook"
sync_backend = "file"

[file]
remote_path = "/Volumes/shared/brew-sync.toml"
EOF

# Pull and apply
brew-sync pull
brew-sync status          # review what will change
brew-sync apply           # install missing packages, upgrade outdated ones
brew-sync reconcile       # add local-only packages to the manifest
brew-sync push            # push updated manifest back
```

### Regular sync between machines

```bash
# On any machine — pull latest, review, apply
brew-sync pull
brew-sync status
brew-sync apply
```

### After installing new packages manually

```bash
# Push updated state to remote
brew-sync push
```

Other machines can then `pull` and `apply` to pick up the new packages.

### Safe review before applying

```bash
# See what would change
brew-sync status --verbose

# Or dry-run the apply
brew-sync apply --dry-run

# If it looks good, apply for real
brew-sync apply

# Then reconcile any local-only packages
brew-sync reconcile
```

## Troubleshooting

### "brew not found"

brew-sync requires Homebrew. Install it from [https://brew.sh](https://brew.sh):

```bash
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
```

### "manifest not found"

The manifest file doesn't exist yet. Either:

```bash
# Generate from your current packages
brew-sync init

# Or pull from a remote
brew-sync pull
```

### Partial apply failures

If some packages fail during `apply`, brew-sync continues with the rest and reports failures. Re-run `brew-sync apply` to retry — the operation is idempotent. Only failed packages will be retried.

### Sync backend unreachable

Check your config file and ensure:

- **Git**: The repo URL is correct and you have SSH/HTTPS access
- **File**: The path exists and you have read/write permissions

```bash
# Verify your config
cat ~/.config/brew-sync/config.toml
```

### Validation errors

If the manifest has issues, brew-sync reports all errors at once so you can fix them in a single pass:

```
validation failed: duplicate formula: git; cask entry has empty name; invalid tap format: badtap
```

Edit `brew-sync.toml` to fix the reported issues.
