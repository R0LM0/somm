# Dynamic Config Paths Specification

## Purpose

Ensure all setup and runtime paths are resolved from standard environment functions rather than hardcoded user directories.

## Requirements

### Requirement: OpenCode Config Path

The system MUST locate `opencode.json` at `filepath.Join(os.UserHomeDir(), ".config", "opencode", "opencode.json")`.

#### Scenario: Home dir resolves

- GIVEN a valid home directory
- WHEN `findOpenCodeConfig()` is called
- THEN it MUST return the path above if the file exists

### Requirement: Binary Path

The system MUST locate the `somm` binary in the following order:

1. `filepath.Join(os.Getenv("GOPATH"), "bin", "somm[.exe]")`
2. If `GOPATH` is empty, `filepath.Join(os.UserHomeDir(), "go", "bin", "somm[.exe]")`

If the binary is not found, the wizard MAY prompt the user for the path.

#### Scenario: GOPATH set

- GIVEN `GOPATH` is set to `/custom/go`
- WHEN `findBinary()` is called
- THEN it MUST return `/custom/go/bin/somm`

### Requirement: `.env` Path

The system MUST store and load `.env` next to the resolved binary path.

#### Scenario: Binary in GOPATH/bin

- GIVEN `findBinary()` resolves to `~/go/bin/somm`
- WHEN `.env` is saved or loaded
- THEN the path MUST be `~/go/bin/.env`

### Requirement: No Hardcoded User Paths

No path construction in `cmd/somm` or `internal/api` packages MAY contain hardcoded user names or platform-specific home directories.

#### Scenario: User-independent path

- GIVEN the username is changed
- WHEN paths are resolved
- THEN they MUST still resolve to the current user's home directory
