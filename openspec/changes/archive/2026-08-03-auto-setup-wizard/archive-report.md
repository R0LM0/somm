# Archive Report: auto-setup-wizard

**Change**: auto-setup-wizard
**Archived**: 2026-08-03
**Archive path**: `openspec/changes/archive/2026-08-03-auto-setup-wizard/`
**Mode**: hybrid (Engram + openspec files)
**Status**: success — intentional full archive, no warnings

## Summary

`somm` no longer dies on first run when API keys are missing. A pre-flight config check in `cmd/somm/main.go` (`configReady()` / `maybeRunSetup()`) detects a missing `OPENCODE_API_KEY` and auto-launches the setup wizard when stdin is a terminal, or fails loudly (with a `--skip-setup` hint) when it is not. The wizard (`cmd/somm/setup.go`) was refactored into status mode, provider multi-select (OpenCode required, OpenRouter/Kimi optional), per-provider key prompts, and persistence of `.env` (next to the resolved binary) plus `opencode.json` MCP entry. Config/binary/`.env` paths are now resolved dynamically via `os.UserHomeDir()` and `GOPATH` instead of hardcoded directories. An optional `KIMI_API_KEY` is stored on `api.Client` and reported by `ValidateConfig` (not yet consumed by tools).

## Final State (at close)

| Fact | Value | Source |
|------|-------|--------|
| Verification verdict | **PASS** | `verify-report.md` YAML frontmatter (`verdict: pass`) + final Verdict section + launch prompt |
| Scenarios compliant | 12/17 (2 partial — manual TTY verification; 3 untested — need TTY mocking) | `verify-report.md` compliance matrix |
| Implementation tasks | 32/32 complete (`[x]`) | `tasks.md` (inspected pre-archive) |
| TDD cycle evidence | Present — RED/GREEN/triangulation/safety-net per task (23 rows) | `apply-progress.md` (exists at close) |
| Tests | `go test -cover ./...` → exit 0 — cmd 19.0%, api 88.8%, guide 64.1%, profile 83.7% | verify-report hash + re-run at archive time |
| Vet | `go vet ./...` → exit 0 | verify-report + re-run at archive time |
| Native review | `disabled/unmanaged` — no review artifacts (transaction/ledger/receipt) exist for this change; no native review governs it | no `review/` dir, no `state.yaml` |

## Source-of-Truth Specs Synced

No main specs existed under `openspec/specs/` before this archive, so each delta spec was a full spec and was copied directly (no destructive merge; no requirements removed/renamed):

| Domain | Action | Requirements added |
|--------|--------|--------------------|
| `auto-setup` | Created `openspec/specs/auto-setup/spec.md` | 4 (Pre-flight Config Check, Skip-Setup Flag, Re-evaluate After Wizard) |
| `setup-wizard` | Created `openspec/specs/setup-wizard/spec.md` | 6 (Status Mode, Provider Selection, Key Prompts, Persistence, Reconfiguration with `--force`) |
| `dynamic-paths` | Created `openspec/specs/dynamic-paths/spec.md` | 4 (OpenCode Config Path, Binary Path, `.env` Path, No Hardcoded User Paths) |
| `api-client` | Created `openspec/specs/api-client/spec.md` | 3 (Optional Kimi Key, Environment Loading, Config Validation Status) |

## Archive Contents

- `proposal.md` ✅ (intent, scope, 4 capability areas, risks, rollback)
- `specs/{auto-setup,setup-wizard,dynamic-paths,api-client}/spec.md` ✅ (4 delta specs, 17 scenarios)
- `design.md` ✅ (5 architecture decisions, sequence diagram, interfaces, testing strategy)
- `tasks.md` ✅ (32/32 tasks complete)
- `apply-progress.md` ✅ (TDD cycle evidence, changed files, verification evidence)
- `verify-report.md` ✅ (verdict: pass)
- `archive-report.md` ✅ (this file)

## Evidence Reconciliation (Final-State Authority)

Two `verify-report.md` body statements are stale snapshots that disagree with the final state; the launch prompt and on-disk artifacts outrank them:

1. **`Status: FAIL` header** — verify-report body line 22 says `FAIL`, but the YAML frontmatter (`verdict: pass`), the final Verdict section ("PASS — all requested commands passed..."), and the launch prompt all state PASS. The body header is an outdated draft line not updated when the verdict flipped. Reported final state: **PASS**.

2. **"Apply progress | Missing" / TDD Compliance 1/6** — the verify-report completeness table recorded `apply-progress` as missing and TDD compliance at 1/6. At close, `apply-progress.md` exists in the change folder with a full TDD Cycle Evidence table (RED/GREEN per task 1.1–4.4, changed files, verification evidence). The launch prompt confirms it. Reported final state: **TDD cycle evidence present**. The verify-report's TDD findings describe the moment of verification, before the apply-progress artifact was produced.

Both contradictions are recorded here rather than silently resolved; they do not change the PASS verdict, which is independently corroborated by the frontmatter, the final verdict text, the launch prompt, and a fresh `go test -cover ./...` + `go vet ./...` run at archive time.

## Engram Traceability

Artifacts persisted in Engram for this change (observation IDs):

| Artifact | Observation ID |
|----------|----------------|
| `sdd/auto-setup-wizard/proposal` | #1522 |
| `sdd/auto-setup-wizard/specs` | #1525 |
| `sdd/auto-setup-wizard/design` | #1523 |
| `sdd/auto-setup-wizard/tasks` | #1524 |
| `sdd/auto-setup-wizard/apply-progress` | (filesystem only — no Engram observation) |
| `sdd/auto-setup-wizard/verify-report` | (filesystem only — no Engram observation) |
| `sdd/auto-setup-wizard/archive-report` | (this archive — persisted at close) |

## Gating Checks

- **Task Completion Gate**: ✅ passed — all 32 implementation tasks checked in the persisted `tasks.md` before spec sync; no stale unchecked tasks.
- **Native Review Receipt Gate**: ✅ passed via `disabled/unmanaged` — no review governance exists for this change (no transaction/ledger/receipt artifacts; kill switch off), so no terminal receipt is required.
- **Action Context Guard**: no `workspace-planning` mode or `allowedEditRoots` restrictions were in effect; operations stayed within the repo.

## Known Residuals (non-blocking, carried from verify-report)

1. Changed-file coverage low in `cmd/somm/main.go` (17.2%) and `setup.go` (20.1%) — interactive/server flows not exercised.
2. Three scenarios (interactive wizard, cancel path) require TTY mocking for automated coverage.
3. Bounded verification tracked 550 changed lines against the 400-line budget (documentation/tracking note only).

## SDD Cycle Complete

The change is fully planned, implemented, verified, and archived. Ready for the next change.
