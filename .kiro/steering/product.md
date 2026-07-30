---
inclusion: always
---

# brew-sync Product Context

## What It Does

brew-sync synchronizes Homebrew packages across multiple macOS machines using a declarative TOML manifest (`brew-sync.toml`). You describe the packages you want, and brew-sync handles installing, upgrading, and removing packages to match.

## Core Concepts

- **Manifest** (`brew-sync.toml`) — declares desired formulae, casks, and taps
- **Machine tags** — `machine_tag` in config identifies each machine for per-machine filtering
- **only_on / except_on** — per-package machine filters
- **Sync backends** — git or file-based manifest sharing between machines
- **Deprecated / Obsolete** — auto-detected packages that Homebrew no longer supports

## Commands

| Command | Purpose |
|---------|---------|
| `init` | Generate manifest from current Homebrew installation |
| `status` | Show drift between manifest and local packages |
| `apply` | Install missing, upgrade outdated; optionally remove extras |
| `reconcile` | Interactive walk-through of local-only packages |
| `merge` | Non-interactive union of local state into manifest |
| `push` | Save manifest to sync backend |
| `pull` | Fetch manifest from sync backend |

## Architecture

```
CLI (cobra)
  │
  ├─ config      loads ~/.config/brew-sync/config.toml
  ├─ manifest    reads/writes/validates brew-sync.toml
  ├─ diff        computes install/upgrade/remove operations
  │   ├─ engine  compares manifest vs local state
  │   ├─ filter  applies machine_tag filtering (only_on/except_on)
  │   └─ apply   executes operations via brew runner
  ├─ brew        Homebrew CLI abstraction (Runner interface + mock)
  └─ sync        push/pull backends (git, file) via factory
```

## Key Design Decisions

- Only tracks explicitly installed packages (not dependencies) — uses `brew leaves`
- Manifest is the source of truth; local state is compared against it
- Operations are idempotent — re-running apply retries only what failed
- Deprecated/obsolete packages are auto-marked in manifest and skipped
- brew.Runner interface allows full unit testing without Homebrew installed
- Property-based tests verify invariants (diff correctness, manifest round-tripping)

## User Config

Located at `~/.config/brew-sync/config.toml`:
- `manifest_path` — where to find/write the manifest
- `machine_tag` — this machine's identity for filtering
- `sync_backend` — "git" or "file"
- `[git]` / `[file]` — backend-specific settings
