# note-searcher Development Environment

This document describes the development environment for agents and contributors working on note-searcher.

## Quick Start

### Entering the Development Shell

```bash
nix develop
```

This enters a reproducible development environment with all required tools pre-configured.

### Shell Contents

The `nix develop` shell provides:

- **go** — Go compiler (latest from nixos-unstable)
- **gopls** — Go Language Server Protocol implementation
- **gotools** — Go analysis tools (includes `goimports`)
- **go-tools** — Additional Go tools (includes `staticcheck`)
- **govulncheck** — Go vulnerability checker
- **just** — Task runner (preferred command interface)
- **gomod2nix** — Go module to Nix dependency converter

### Available Just Targets

All commands below must be run inside `nix develop`:

| Target | Purpose |
|--------|---------|
| `just build` | Compile the binary |
| `just test` | Run all tests |
| `just lint` | Run staticcheck and govulncheck |
| `just fmt` | Format code (goimports + gofmt) |
| `just gomod2nix` | Regenerate gomod2nix.toml from go.mod |

Run `just --list` to see all targets.

## Agent Conventions

### 1. Always Use `nix develop`

- Run all builds, tests, and linting **inside** the Nix shell
- Never invoke raw `go`, `staticcheck`, or `govulncheck` commands outside the shell
- This ensures reproducible, consistent tooling across all environments

### 2. Dependency Management

After any change to `go.mod` or `go.sum`:

```bash
just gomod2nix
```

This regenerates `gomod2nix.toml`. Commit the updated file alongside your `go.mod`/`go.sum` changes.

**Example commit:**
```
feat(deps): add encoding/json wrapper package

- Add github.com/pkg/json v1.0.0 as dependency
- Regenerate gomod2nix.toml
```

### 3. No GitHub Actions in This Issue

Do not introduce CI workflows or GitHub Actions in issue #16. The dev environment is local-only for now.

### 4. Use `justfile` as the Stable Interface

The `justfile` is the official command interface for this project. Prefer `just <target>` over raw tool invocations.

- `just build` over `go build ./...`
- `just test` over `go test ./...`
- `just lint` over `staticcheck ./...` + `govulncheck ./...`

This maintains a consistent, versioned interface as the project grows.

## Repository Structure

The repository follows a standard Go project layout:

```
note-searcher/
├── cmd/                    # Command-line entrypoints
│   └── note-searcher/      # Main CLI binary
│       └── main.go
├── internal/               # Private packages (not importable from outside)
│   ├── index/              # Indexing logic and backends
│   ├── search/             # Search query and result handling
│   ├── config/             # Configuration parsing
│   └── format/             # Output formatting
├── pkg/                    # Public packages (for future library consumers)
│   └── (added as needed)
├── testdata/               # Test fixtures and data
├── go.mod                  # Go module declaration
├── go.sum                  # Go module checksums (auto-generated)
├── gomod2nix.toml          # Nix dependency snapshot
├── justfile                # Task definitions
├── flake.nix               # Nix development environment
└── design.md               # Implementation specification (source of truth)
```

For details on what each command does and how the tool works, see [design.md](./design.md).

## Design Reference

[design.md](./design.md) is the source of truth for:

- Note schema and frontmatter structure
- Command specifications (`index`, `search`, `probe`, `get`, `batch`)
- Output formats and filtering behavior
- MVP requirements and future nice-to-haves

Always consult design.md before implementing features.

## Building and Testing

Inside `nix develop`:

```bash
# Compile the binary
just build

# Run tests
just test

# Run linters
just lint

# Format code
just fmt

# Check code style only (without auto-fixing)
gofmt -d .
staticcheck ./...
```

## Troubleshooting

### "command not found: go"

You're not inside the Nix shell. Run:
```bash
nix develop
```

### "gomod2nix not found"

Same issue — enter the shell first:
```bash
nix develop
just gomod2nix
```

### Flake lock is stale

If you've added dependencies or changed flake inputs, regenerate the lock file:
```bash
nix flake update
```

This requires `nix` CLI to be available on your system (installed via NixOS, nix-darwin, or Nixpacks).
