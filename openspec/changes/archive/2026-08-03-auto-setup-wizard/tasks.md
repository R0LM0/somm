# Tasks: Auto-Setup Wizard

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~180 |
| 400-line budget risk | Low |
| Chained PRs recommended | No |

## Phase 1: Config Pre-flight in `cmd/somm/main.go`

- [x] 1.1 Add `configReady()` helper that loads `.env` and checks `OPENCODE_API_KEY`.
- [x] 1.2 Add `isTerminal()` helper using `term.IsTerminal(int(os.Stdin.Fd()))`.
- [x] 1.3 Add `--skip-setup` flag parsed before the main flag set.
- [x] 1.4 Implement `maybeRunSetup()`: if missing key and terminal, run `runSetup()` then re-check; if key is now present, print "Setup complete! Starting server..." and return true (caller continues to server startup); if key still missing or wizard failed, return false; if missing and not terminal, fatal error with `--skip-setup` hint; if `--skip-setup`, fatal error.
- [x] 1.5 Wire `maybeRunSetup()` at the start of `main` before `run()`. If it returns false, exit; if true, continue to `run()` which starts the MCP server.

## Phase 2: Setup Wizard Flow in `cmd/somm/setup.go`

- [x] 2.1 Parse `--force` flag in `runSetup()`.
- [x] 2.2 Implement `isAlreadyConfigured()` checking `.env` for `OPENCODE_API_KEY` and `opencode.json` for a `somm` MCP entry.
- [x] 2.3 Implement `runStatus()` printing status and the tool list.
- [x] 2.4 Implement provider multi-select using `huh.NewMultiSelect`: OpenCode required and selected, OpenRouter optional, Kimi optional.
- [x] 2.5 Implement key prompts for each selected provider, validating that OpenCode is non-empty.
- [x] 2.6 Save `.env` with `OPENCODE_API_KEY`, `OPENROUTER_API_KEY`, and `KIMI_API_KEY` as selected.
- [x] 2.7 Update `opencode.json` MCP entry with the resolved binary path.
- [x] 2.8 Keep existing friendly labels but ensure the flow follows the new requirements.

## Phase 3: Dynamic Path Resolution

- [x] 3.1 Ensure `findOpenCodeConfig()` uses `os.UserHomeDir()`.
- [x] 3.2 Ensure `findBinary()` uses `os.Getenv("GOPATH")` with fallback to `os.UserHomeDir()/go/bin`.
- [x] 3.3 Ensure `.env` save/load path is always derived from the resolved binary path.
- [x] 3.4 Audit `cmd/somm` and `internal/api` for any hardcoded user paths.

## Phase 4: Kimi API Key Support

- [x] 4.1 Add `KIMAPIKey` field to `internal/api/models.go` `Client` struct.
- [x] 4.2 Update `internal/api/api.go` `NewClient` signature to accept `kimiKey`.
- [x] 4.3 Update `cmd/somm/main.go` to read `KIMI_API_KEY` from env and pass it to `NewClient`.
- [x] 4.4 Update `internal/api/validate.go` to report Kimi as configured when the key is present.

## Phase 5: Tests

- [x] 5.1 Update `cmd/somm/main_test.go` `TestServerRequiresOpenCodeKey` to pass `--skip-setup` and assert the same failure.
- [x] 5.2 Create `cmd/somm/setup_test.go` with table tests for `isAlreadyConfigured()`, provider selection validation, and `.env` persistence.
- [x] 5.3 Add unit tests for `configReady()` with temp `.env` and env vars.
- [x] 5.4 Add unit tests for `findBinary()` and `findOpenCodeConfig()` using temp home/GOPATH.
- [x] 5.5 Add integration test or manual verification that after wizard completes, the server starts automatically without requiring a restart.
- [x] 5.6 Run `go test -cover ./...` and fix lint issues.

## Phase 6: Integration and Verify

- [x] 6.1 Run `go build ./cmd/somm` and manual terminal check that missing keys launch the wizard.
- [x] 6.2 Confirm that after wizard completes, the server starts automatically (no manual restart needed).
- [x] 6.3 Confirm `somm setup` with existing config shows status.
- [x] 6.4 Confirm `somm setup --force` re-runs prompts.
- [x] 6.5 Run `go test -cover ./...` and `go vet ./...`.
