# Tasks: Provider-Agnostic Role Profiles

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~520 (authored, goldens excluded) |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 (profile package) -> PR 2 (recommend.go rework + wiring + parity) |
| Delivery strategy | auto-chain |
| Chain strategy | stacked-to-main |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: High

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | Standalone `internal/profile/` package: schema, loader, resolver, embedded `gentle-ai.yaml`, unit tests | PR 1 | `go test ./internal/profile/...` | N/A — pure library, no wiring into `main.go` yet, verified by unit tests only | Delete `internal/profile/` package and revert `go.mod` yaml.v3 line; no other file touched |
| 2 | Capture golden baseline; rework `recommend.go` scoring; wire `Profile` through `main.go`/`guide`; golden parity test | PR 2 | `go test ./internal/api/... ./internal/guide/...` | `go run ./cmd/somm recommend_config` against mock/fixture OC+OR data, compare to golden | Revert PR 2 commits only; PR 1's `internal/profile` package stays intact and unused, `AllAgentRoles`/`BestMetric` path restored |

## Phase 1 (PR 1): `internal/profile` Package — Foundation

- [x] 1.1 Add `gopkg.in/yaml.v3` to `go.mod`/`go.sum`.
- [x] 1.2 Create `internal/profile/profile.go`: `Profile`, `Role`, `RoleDefaults` types; metric key constants (`intelligence`, `coding`, `agentic`).
- [x] 1.3 Create `internal/profile/load.go`: YAML parse via `yaml.v3`, schema validation (unknown metric key, missing `id` → error), defaults-merge (`min_context`/`max_input_price`/`requires` inherit from `Defaults` when role-level is nil; `weights` never defaulted).
- [x] 1.4 Create `internal/profile/resolve.go`: `Resolve()` implementing flag -> `SOMM_PROFILE` env -> `./somm.yaml` -> XDG config path -> embedded default order; malformed file at any selected source returns a fatal error (no silent fallback).
- [x] 1.5 Create `internal/profile/preset.go` with `//go:embed presets/gentle-ai.yaml`.
- [x] 1.6 Create `internal/profile/presets/gentle-ai.yaml`: all 19 roles from today's `AllAgentRoles()`, each former `BestMetric` mapped to a single `weights` entry at `1.0` (except `cheapest` roles → `weights: {}`), Spanish `Criteria` preserved as `description`.

## Phase 2 (PR 1): Unit Tests — Profile Package

- [x] 2.1 `internal/profile/load_test.go`: table test — valid multi-metric profile loads with both weights and hard constraints populated (role-profiles spec, "Valid multi-metric profile loads").
- [x] 2.2 `internal/profile/load_test.go`: table test — unknown metric key (e.g. `creativity`) fails loading with a validation error naming the key ("Unknown metric key is rejected").
- [x] 2.3 `internal/profile/load_test.go`: table test — role missing `id` fails validation ("Missing `id` on a role fails validation").
- [x] 2.4 `internal/profile/load_test.go`: table test — defaults merge: role without `min_context` inherits `defaults.min_context`; role that sets its own value overrides the default; `weights` is never defaulted, even when omitted ("Role inherits default hard constraints" / "Role overrides a default" / "Weights are never defaulted").
- [x] 2.5 `internal/profile/resolve_test.go`: env + tmpfile test — flag wins over `SOMM_PROFILE` and `./somm.yaml` when all three are present ("Flag wins over env and file").
- [x] 2.6 `internal/profile/resolve_test.go`: no flag/env/file/XDG present → embedded `gentle-ai` preset loads, treated as success not error ("No external source present falls through to embedded default").
- [x] 2.7 `internal/profile/resolve_test.go`: malformed YAML at the selected source (e.g. `./somm.yaml`) → `Resolve` returns an error ("Malformed profile fails loud, no silent fallback").
- [x] 2.8 `internal/profile/preset_test.go`: embedded `gentle-ai.yaml` parses cleanly via the same loader used for external files; asserts 19 roles present.

## Phase 3 (PR 2): Golden Baseline Capture (MUST run before any scoring change)

- [x] 3.1 On the current (pre-refactor) `internal/api/recommend.go` behavior, run a fixed set of mock OpenRouter/OpenCode candidate models through `FormatRecommendations` and check in the byte-for-byte output as `internal/api/testdata/gentle-ai.golden`.
- [x] 3.2 Commit the golden fixture and the mock candidate model inputs used to produce it as a standalone commit, before touching `recommend.go` scoring logic.

## Phase 4 (PR 2): Weighted Scoring Rework — `internal/api/recommend.go`

- [x] 4.1 Replace `AllAgentRoles()` role source with `[]profile.Role` from the active `*profile.Profile`.
- [x] 4.2 Implement hard-constraint pre-filter in this exact order: (1) skip no-pricing models, (2) skip models at assignment cap, (3) apply `min_context`/`max_input_price`/`requires`/`exclude_family_of`, (4) for weighted roles skip candidates with a `nil` value on any positively-weighted metric (weighted-scoring spec, "Hard Constraint Pre-Filter").
- [x] 4.3 Implement `requires: ["reasoning"]` capability-token matching against `model.Reasoning != nil` (role-profiles spec, "`requires: [\"reasoning\"]` filters non-reasoning models").
- [x] 4.4 Implement `exclude_family_of: <role-id>` using `deriveProvider(ocID)` to exclude candidates whose family matches the family already assigned to `<role-id>` this run (weighted-scoring spec, "`exclude_family_of` excludes a family already assigned").
- [x] 4.5 Compute `Qraw = Σ(weight_m * raw_m)` per candidate per role; ratio = `Qraw / price`, tiebreak `Qraw` desc — never a normalized value in the ratio (design Decision 1).
- [x] 4.6 Implement `Qnorm` normalization (`norm_m = (raw_m - min_m)/(max_m - min_m)`, neutral `1.0` on zero range) that engages only for roles weighting >=2 metrics, over that role's constraint-filtered candidate set; `Qnorm` used only as internal comparison, never replacing `Qraw` in the ratio (design Decision 2).
- [x] 4.7 Implement empty-`weights` price-minimizing objective: sort by price asc, tiebreak intelligence desc with `50.0` fallback for null intelligence (design Decision 3).
- [x] 4.8 Preserve max-2-per-model assignment cap with relax-to-3 fallback; hard constraints never relax; still-empty set after relaxation surfaces today's per-role "no model available" reason path, not a fatal error (weighted-scoring spec, "Empty Candidate Set Fallback" / "Assignment Spreading Preserved").
- [x] 4.9 Update `buildReason` to show the dominant-weight metric for weighted roles.
- [x] 4.10 Update `RecommendConfig` signature to `RecommendConfig(ctx, client, prof *profile.Profile, roleFilter []string)`.

## Phase 5 (PR 2): Wiring — Callers and Guide

- [x] 5.1 Update `internal/api/cost.go` to pass `*profile.Profile` through the new `RecommendConfig` signature.
- [x] 5.2 Update `internal/api/export.go` to pass `*profile.Profile` through the new `RecommendConfig` signature.
- [x] 5.3 Update `internal/api/validate.go` to pass `*profile.Profile` through the new `RecommendConfig` signature.
- [x] 5.4 Update `internal/guide/guide.go` so role descriptions are sourced from the active `*profile.Profile` instead of the hardcoded `guia_gentle_ai.md` text; demote that markdown to the `gentle-ai` preset's doc.
- [x] 5.5 Update `cmd/somm/main.go`: add `--profile` flag and `SOMM_PROFILE` env wiring, call `profile.Resolve()` at startup (fatal on error, per role-profiles "Fail-Loud Validation"), thread the resolved `*profile.Profile` into all `RecommendConfig` and `get_agent_criteria` call sites (8 callers total across `main.go` and `internal/api/*.go`).

## Phase 6 (PR 2): Tests — Scoring, Constraints, Parity

- [x] 6.1 `internal/api/recommend_test.go`: table test — raw-ratio ordering matches pre-refactor behavior using the design's worked example (A: intelligence=72.5, price=30; B: intelligence=52.1, price=0.95 → B wins) (weighted-scoring spec, "Raw ratio ordering matches pre-refactor behavior").
- [x] 6.2 `internal/api/recommend_test.go`: table test — `min_context` and `max_input_price` filter candidates correctly, both bounds independently ("`min_context` and `max_input_price` filter candidates").
- [x] 6.3 `internal/api/recommend_test.go`: table test — `exclude_family_of` excludes a family already assigned to another role in the same run.
- [x] 6.4 `internal/api/recommend_test.go`: table test — single-candidate and all-equal-candidates normalization edge cases both yield `norm_m == 1.0` (no div-by-zero) ("Single candidate normalizes to neutral value" / "All-equal candidates normalize to neutral value").
- [x] 6.5 `internal/api/recommend_test.go`: table test — 2-to-3 cap relaxation recovers a candidate when the only blocker was the assignment cap, and does not recover one when hard constraints alone empty the set ("Cap relaxation recovers a candidate" / "Constraints alone empty the set, no relaxation recovers it").
- [x] 6.6 `internal/api/recommend_test.go`: table test — no single model assigned to more than 3 roles across a full run ("A model is not assigned to more than 3 roles").
- [x] 6.7 `internal/api/recommend_test.go`: table test — empty-`weights` cheapest role sorts by price ascending, and null intelligence resolves to `50.0` and wins the tiebreak over a `40.0` candidate (gentle-ai-preset spec, "Cheapest role sorts by price ascending" / "Null intelligence falls back to 50.0 for tiebreak").
- [x] 6.8 `internal/api/recommend_test.go`: golden parity test — run the embedded `gentle-ai` preset against the fixed mock OC/OR candidates from Phase 3 and assert `FormatRecommendations` output matches `testdata/gentle-ai.golden` byte-for-byte. Depends on task 3.1 (baseline must exist first).
- [x] 6.9 `internal/api/recommend_test.go`: unit test — every `gentle-ai` preset role has exactly one weight at `1.0`; normalization never engages for it and `Qraw` equals the single raw benchmark value (gentle-ai-preset spec, "Single-weight roles bypass normalization").

## Phase 7 (PR 2): Integration and Cleanup

- [x] 7.1 Run `go build ./...` and `go vet ./...` across the repo to confirm no remaining `AllAgentRoles`/`BestMetric` callers and no signature mismatches.
- [x] 7.2 Run `go test ./...` full suite; confirm golden parity test (6.8) passes byte-for-byte against `testdata/gentle-ai.golden`.
- [x] 7.3 Update any developer-facing docs/comments referencing `AllAgentRoles()` or `BestMetric` to describe the new `profile.Profile`-driven flow.
