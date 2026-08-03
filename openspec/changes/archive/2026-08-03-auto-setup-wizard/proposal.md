# Proposal: Auto-Setup Wizard

## Intent

When users run `somm` for the first time without API keys, the binary currently exits with a fatal error. The intended first-run experience is to guide them through configuration interactively, as `somm setup` already does. This change auto-launches that wizard when required config is missing and improves the wizard so it skips redundant steps when the user is already configured.

## Scope

### In Scope
- Auto-detect missing `OPENCODE_API_KEY` in `main` and launch the setup wizard instead of failing.
- Refactor the setup wizard to:
  - Show status and skip key entry when `.env` and `opencode.json` are already valid.
  - Ask which providers the user has (OpenCode required, OpenRouter optional, Kimi optional).
  - Prompt for each selected provider's key.
  - Persist `.env` and update `opencode.json`.
- Use `os.UserHomeDir()` and `os.Getenv("GOPATH")` for all config/binary paths.
- Add optional `KIMI_API_KEY` plumbing from `.env` to `api.Client`.

### Out of Scope
- Changing MCP tool behavior or recommendation logic.
- Full Kimi API integration beyond storing and reading the key.
- Non-interactive auto-installation (CI must use `--skip-setup` or pre-set env vars).

## Capabilities

### New Capabilities
- `auto-setup`: pre-flight config detection in `cmd/somm/main.go` and conditional wizard launch.
- `setup-wizard`: provider selection, per-provider key prompts, status mode, and persistence.
- `dynamic-config-paths`: home/GOPATH-based resolution of `opencode.json`, binary, and `.env`.

### Modified Capabilities
- `api-client`: accept optional `KIMI_API_KEY` and expose it as a configured provider.

## Approach

Add a `configReady()` helper in `main.go` that checks `OPENCODE_API_KEY` after the same `.env` loading used today. If not ready and stdin is a terminal, run `runSetup()` and then re-check; if still not ready, exit with a clear message. If stdin is not a terminal, fail with the same actionable message so scripts do not hang. Refactor `setup.go` into status detection, provider selection, key prompts, and save phases, reusing `findBinary()` and `findOpenCodeConfig()` for dynamic paths.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `cmd/somm/main.go` | Modified | Pre-flight config check, `--skip-setup` flag, auto-launch call |
| `cmd/somm/setup.go` | Modified | Status mode, provider multi-select, Kimi key, save paths |
| `internal/api/models.go` | Modified | Add `KIMAPIKey` field to `Client` |
| `internal/api/api.go` | Modified | `NewClient` accepts Kimi key; `ValidateConfig` reflects it |
| `cmd/somm/main_test.go` | Modified | Use `--skip-setup` for missing-key test |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| CI/tests hang on interactive prompt | Low | Detect non-terminal stdin; add `--skip-setup`; update tests |
| `.env` path differs between dev and installed binary | Med | Always save `.env` next to the resolved binary path; load from binary dir first |
| Breaking existing `somm setup` behavior | Low | Preserve existing valid-config status path; gate new flow with `--force` |

## Rollback Plan

Revert the change. `main.go` returns the original fatal error when `OPENCODE_API_KEY` is missing. No data migration is needed.

## Dependencies

- `charmbracelet/huh` (already in `go.mod`).

## Success Criteria

- [ ] `somm` without args and without keys opens the setup wizard in a terminal.
- [ ] `somm` without keys in a non-terminal exits with a clear error, no hang.
- [ ] `somm setup` with existing config prints status and skips prompts.
- [ ] `somm setup --force` re-runs provider/key prompts.
- [ ] `go test -cover ./...` passes.

## Proposal Question Round (Assumptions)

1. Non-interactive/CI should fail loudly rather than auto-launch. Acceptable?
2. After the wizard finishes, should `somm` immediately start the server, or exit and require a restart?
3. Is "Kimi" currently a separate key/env var, or should selecting Kimi simply set `KIMI_API_KEY` for future use?
