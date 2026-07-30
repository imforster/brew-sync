---
inclusion: always
---

# brew-sync Project Conventions

## Language & Tooling

- Go 1.25+ (module: `brew-sync`)
- Build: `make build` (output: `build/brew-sync`)
- Test: `make test` or `go test ./...`
- Property tests: `make test-property` (uses `pgregory.net/rapid`)
- Lint: `make lint` (golangci-lint)
- Format: `make fmt` (gofmt)
- Race detection: `make test-race`
- CI: GitHub Actions on macOS (push/PR to main)

## Project Structure

```
main.go              → entrypoint, delegates to cmd.Execute()
cmd/                 → cobra commands (init, status, apply, reconcile, merge, push, pull)
internal/
  config/            → TOML config loading (~/.config/brew-sync/config.toml)
  manifest/          → manifest parsing, validation, read/write (brew-sync.toml)
  diff/              → diff engine, filtering, apply logic
  brew/              → Homebrew CLI runner (abstracted behind interface for testing)
  sync/              → sync backends (git, file) with factory pattern
```

## Code Style

- Standard Go conventions (gofmt, go vet)
- Cobra for CLI commands with `--verbose`, `--dry-run`, `--remove` flags
- BurntSushi/toml for TOML parsing
- Interfaces for testability (e.g., `brew.Runner` interface with mock)
- `internal/` packages are not exported — all public API is via CLI
- Error wrapping with `fmt.Errorf("context: %w", err)`
- Table-driven tests preferred
- Property-based tests with `pgregory.net/rapid` for invariant checking

## Testing Patterns

- Unit tests: `*_test.go` alongside source
- Property tests: `*_property_test.go` files using rapid
- Integration tests: `cmd/integration_test.go` and `cmd/cli_integration_test.go`
- Mock Homebrew runner for isolated testing (`internal/brew/mock_runner.go`)
- Test coverage: `make test-cover`

## Dependencies

Keep dependencies minimal. Current set:
- `github.com/BurntSushi/toml` — TOML parsing
- `github.com/spf13/cobra` — CLI framework
- `pgregory.net/rapid` — property-based testing

Do not add new dependencies without justification. Prefer stdlib.

## Build & Version

- Version injected via ldflags at build time (`cmd.Version`, `cmd.Commit`)
- Binary name: `brew-sync`
- Build directory: `build/`
