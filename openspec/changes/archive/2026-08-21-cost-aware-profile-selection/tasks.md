# Tasks: Cost-Aware Profile Selection

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~955 authored (goldens excluded): PR1 ~355, PR2a ~380, PR2b ~220 |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR1 (`selection.objective`) -> PR2a (`selection.currency: quota`) -> PR2b (tier capture wizard) |
| Delivery strategy | ask-on-risk |
| Chain strategy | stacked-to-main |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: High

Split, chain strategy, and per-PR scope were already decided by the user (see design's Review Budget
section) and are not re-derived here. `Decision needed before apply: Yes` reflects the cached
`ask-on-risk` delivery strategy — `sdd-apply` still confirms proceeding with this exact 3-slice plan
before starting PR1.

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | `selection.objective` schema, validation, `value`/`quality`/`budget` comparators, `Reason` arms, golden parity unmodified | PR1 | `go test ./internal/profile/... ./internal/api/...` | N/A — no runtime/network surface; MCP tool wiring unchanged, proven entirely by unit + golden tests | Revert PR1 commits; `internal/profile/profile.go`/`load.go`/`internal/api/recommend.go` return to pre-change state, `presets/gentle-ai.yaml` untouched |
| 2 | `selection.currency: quota`: embedded plans table, saturating affordability comparator, `frequency` field, untabulated fallback, staleness `Reason` | PR2a | `go test ./internal/profile/plans/... ./internal/profile/... ./internal/api/...` | N/A — plans table is embedded (no network/filesystem access outside the binary), proven by unit tests over fixed candidate sets | Revert PR2a commits only; PR1's objective axis stays intact and functional under `usd` currency |
| 3 | Tier capture wizard screen, `ResolveWithTier`/`TierCurrency`, `main.go`/`setup.go` wiring, `.env` persistence | PR2b | `go test ./cmd/somm/... ./internal/profile/...` | `somm setup` (and `somm setup --force`) run interactively against a temp `opencode.json`/`.env` fixture, verifying the tier screen shows/skips/re-asks correctly | Revert PR2b commits only; PR1+PR2a stay functional via explicit `selection.currency: quota` in a profile file, only tier-driven auto-default is lost |

## Phase 1 (PR1): Selection Schema Foundation

- [x] 1.1 In `internal/profile/profile.go`, add `Selection{Objective, Currency string}` (yaml tags `objective,omitempty`/`currency,omitempty`), `Profile.Selection *Selection`, `Role.Selection *Selection`, and unexported `Role.currencyExplicit bool` (role-profiles: Selection Block Schema, Per-Role Selection Override).
- [x] 1.2 In `internal/profile/load.go` `validate()`, reject unknown `objective`/`currency` values, naming the value (role-profiles: Selection Block Schema "Unknown objective value rejected").
- [x] 1.3 In `internal/profile/load.go`, resolve effective selection field-by-field (role -> profile -> `{value, usd}`) during the merge pass, setting `currencyExplicit` when currency was set at role or profile level (role-profiles: Per-Role Selection Override, both scenarios).
- [x] 1.4 In `internal/profile/load.go`, add a post-`mergeDefaults` validation pass: effective `objective == "budget"` with no effective `max_input_price` (role or `defaults`) fails fatal, naming the missing ceiling (role-profiles: Budget Objective Requires an Effective Ceiling, both scenarios; design Decision 5).
- [x] 1.5 In `internal/api/recommend.go` `findBestModel`, move the existing weighted `sort.Slice` closure verbatim into a `case "value":` arm of a new objective switch keyed on `role.Selection.Objective`; byte-identical to today (design Decision 1 & 4).
- [x] 1.6 Add `case "quality":`: sort `Qraw` desc, tiebreak denominator asc, tiebreak `OCID` asc for determinism (weighted-scoring: Objective-Selected Comparator "quality picks top-Qraw"/"ties broken by lower denominator"; design Decision 4).
- [x] 1.7 Add `case "budget":`: reuse the `value` comparator over the set already filtered by `max_input_price` in `collectCandidates` — no new filter code (weighted-scoring: Objective-Selected Comparator "budget never exceeds max_input_price"; design Decision 4).
- [x] 1.8 In `buildReason`, keep the `value`+`usd` `fmt.Sprintf` line unchanged verbatim; add `quality`+`usd` and `budget`+`usd` arms per design Decision 6.

## Phase 2 (PR1): Tests — Selection Schema and Comparators

- [x] 2.1 `internal/profile/load_test.go`: no `selection` block anywhere resolves `objective: value`/`currency: usd` for every role (role-profiles "Default profile has no selection block").
- [x] 2.2 `internal/profile/load_test.go`: unknown `objective` value fails naming it ("Unknown objective value rejected"). (Also triangulated with an unknown `currency` value test, both covered by the same `validateSelection` implementation.)
- [x] 2.3 `internal/profile/load_test.go`: role sets only `objective`, inherits profile `currency`, and vice versa (Per-Role Selection Override, both scenarios).
- [x] 2.4 `internal/profile/load_test.go`: `budget` objective with no effective `max_input_price` fails; `budget` with `defaults`-inherited ceiling loads (Budget Objective Requires an Effective Ceiling, both scenarios).
- [x] 2.5 `internal/api/recommend_test.go`: default objective (no `selection` block) reproduces design's A/B worked example unchanged (weighted-scoring "Raw ratio ordering matches pre-refactor behavior under the default objective").
- [x] 2.6 `internal/api/recommend_test.go`: `quality` picks top-`Qraw` over better ratio; equal-`Qraw` tie broken by lower price (weighted-scoring "quality objective picks top-Qraw"/"ties broken by lower denominator"). (Also triangulated with a full-tie OCID-determinism test.)
- [x] 2.7 `internal/api/recommend_test.go`: `budget` excludes above-ceiling candidates, winner is best `value`-ranked candidate at/below ceiling (weighted-scoring "budget objective never exceeds max_input_price").
- [x] 2.8 `internal/api/recommend_test.go`: `TestRecommendConfig_GoldenParity` (existing test name) runs unmodified, 0 diff lines against `testdata/gentle-ai.golden` (design Decision 1 mechanical proof).
- [x] 2.9 Ran `go build ./...`, `go vet ./...`, `go test ./internal/profile/... ./internal/api/...` — all pass (72/72). `gofmt -w .` intentionally NOT run repo-wide: pre-existing CRLF-vs-LF formatting drift affects ~18 unrelated files across the repo (verified via `gofmt -l .`); all 5 files touched by this PR are already gofmt-clean.

## Phase 3 (PR2a): Plans Package and Quota Comparator

- [x] 3.1 Create `internal/profile/plans/opencode-zen-go.yaml`: `plan`, `measured_at` (`2006-01-02`), `models: ocId -> requests_per_5h` for at least one tier (plan-quota-currency: Embedded Quota Table).
- [x] 3.2 Create `internal/profile/plans/plans.go`: `//go:embed opencode-zen-go.yaml`, `Table{Plan, MeasuredAt, Models}`, `OpenCodeZenGo() (*Table, error)` validating `MeasuredAt` as `2006-01-02`, `(*Table).Requests(ocID) (int, bool)` — `ok=false` for absent, never 0-as-default (plan-quota-currency: Embedded Quota Table both scenarios; design Interfaces).
- [x] 3.3 In `internal/profile/profile.go`, add `Role.Frequency string` (`frequency,omitempty`) and `FrequencyWeight(freq string) float64` (`high`->4.0, `medium`/`""`->1.0, `low`->0.25) (role-profiles: Frequency Field; plan-quota-currency: Frequency Weighting).
- [x] 3.4 In `internal/profile/load.go` `validate()`, reject unknown `frequency` values, naming the value (role-profiles: Frequency Field "Unknown frequency value rejected").
- [x] 3.5 In `internal/api/recommend.go`, add `quota`/`hasQuota` fields to `scoredModel`; in `collectCandidates`, when effective currency is `quota`, resolve `denominator = H_sat / min(rp5h/FrequencyWeight(role.Frequency), H_sat)` (`H_sat=200`) for tabulated candidates, else the bridged `denominator = price / P_min` over the role's constraint-filtered set (plan-quota-currency: Frequency Weighting, Fallback for Untabulated Models; weighted-scoring: Currency-Selected Denominator; design Decision 2 & 3).
- [x] 3.6 Wire the resolved `quota`/`usd` denominator into the PR1 `value`/`quality`/`budget` comparators without altering the `value`+`usd` unedited arm (weighted-scoring: Currency-Selected Denominator).
- [x] 3.7 Update `buildReason`: `quota`-tabulated winners append `measured_at`; untabulated/fallback winners omit any staleness claim (plan-quota-currency: Staleness Surfacing, both scenarios; design Decision 6).
- [x] 3.8 In `RecommendConfig`, load the plans table only when at least one role's effective currency resolves to `quota` — zero I/O otherwise (design Decision 1 parity table).

## Phase 4 (PR2a): Tests — Plans, Quota, Frequency

- [x] 4.1 `internal/profile/plans/plans_test.go`: known `ocId` resolves `requests_per_5h`; unknown `ocId` returns `ok=false`; `measured_at` parses as `2006-01-02` (plan-quota-currency: Embedded Quota Table both scenarios).
- [x] 4.2 `internal/api/recommend_test.go`: Decision 2/3's exact P/C/U worked example at `frequency: high` and `frequency: low`, asserting the ranking inversion and the `H_sat=200` boundary (weighted-scoring "quota currency ranks high-quota cheap model above low-quota premium model"; plan-quota-currency "Higher frequency weight favors quota-abundant models").
- [x] 4.3 `internal/api/recommend_test.go`: untabulated candidate is ranked, never excluded/errored, using the exact `P_min=2.0`/`price=4.0` -> `denominator=2.0` bridge (plan-quota-currency: Fallback for Untabulated Models; weighted-scoring "model missing from quota table uses the bridged denominator").
- [x] 4.4 `internal/api/recommend_test.go`: `Reason` for a tabulated winner contains `measured_at`; `Reason` for a fallback winner omits it (plan-quota-currency: Staleness Surfacing, both scenarios).
- [x] 4.5 `internal/profile/load_test.go`: unknown `frequency` value fails naming it; `frequency` has no ranking effect under `currency: usd` (role-profiles: Frequency Field, both scenarios).
- [x] 4.6 Re-run `TestGoldenParity`: still 0 diff lines against `testdata/gentle-ai.golden` after PR2a (design mechanical proof, both PRs).
- [x] 4.7 Run `go build ./...`, `go vet ./...`, `go test ./internal/profile/... ./internal/api/...`.

## Phase 5 (PR2b): Tier Capture Wizard and Wiring

- [x] 5.1 In `internal/profile/resolve.go`, add `TierCurrency(tier string) (string, error)`: `""`->`"usd"`, `"go"`/`"zen"`->`"quota"`, else error (design Decision 5, Interfaces).
- [x] 5.2 Add `ResolveWithTier(flagPath, tier string) (*Profile, error)`: reuses `Resolve`'s source resolution, then overwrites each role's effective currency only where `!currencyExplicit`; redefine `Resolve(flagPath)` as `ResolveWithTier(flagPath, "")` so its 4 existing callers in `cmd/somm/main.go` are unaffected (design Decision 5).
- [x] 5.3 In `cmd/somm/main.go` `run()`, read `SOMM_OC_TIER`, call `profile.TierCurrency`, fail fatal on an unrecognized value (matching the existing `-opencode-api-key required` style), and call `profile.ResolveWithTier(profilePath, tier)` in place of `profile.Resolve(profilePath)` (design Decision 5, File Changes: main.go).
- [x] 5.4 In `cmd/somm/tui.go`, append `scrTier` last in the `screen` iota block (no renumbering) and add `tiers []string`/`tierCursor int` to `Model` (design Wizard flow).
- [x] 5.5 In `updateProviderKeyInput`, change the `m.keyIdx == len(m.keyFields)-1` branch to enter `scrTier` instead of `scrConfigProgress`; add `updateTier(msg tea.KeyMsg)` handling `up`/`down`, `enter` (sets `m.keys["SOMM_OC_TIER"]`, advances to `scrConfigProgress`), and `esc` (returns to the last key field, no skip) (setup-wizard: Tier Capture "asked once on first setup"; design Wizard flow).
- [x] 5.6 Add a `scrTier` case to `Model.View()` rendering the `go`/`zen` choice, cursor preselecting any tier already in `.env` (setup-wizard: Tier Capture; design Wizard flow preselection).
- [x] 5.7 In `cmd/somm/setup.go`, decide whether `scrTier` is needed independently of the existing `alreadyConfigured` key check: read `SOMM_OC_TIER` from `.env` at `envPath` and skip the screen only when already present and `--force` was not passed — this also covers an existing install upgrading onto this feature (`alreadyConfigured=true` for keys, no persisted tier yet) (setup-wizard: Tier Capture, all three scenarios).
- [x] 5.8 In `saveEnvFile`, append `"SOMM_OC_TIER"` to the `order` allowlist (setup-wizard: Persistence "Save writes the persisted tier").

## Phase 6 (PR2b): Tests — Tier Resolution and Wizard

- [x] 6.1 `internal/profile/resolve_test.go`: `TierCurrency` domain — `""`->usd, `go`/`zen`->quota, unrecognized->error (design Decision 5).
- [x] 6.2 `internal/profile/resolve_test.go`: explicit `selection.currency` beats a non-empty tier passed to `ResolveWithTier` (design Decision 5).
- [x] 6.3 `cmd/somm/tui_test.go`: direct `Model.Update()` with `tea.KeyMsg` (go-testing gate) — `scrTier` entry from the last key field, cursor movement, `enter` sets `SOMM_OC_TIER` and advances, `esc` returns to the last key field (setup-wizard: Tier Capture "asked once"; design Wizard flow).
- [x] 6.4 `cmd/somm/tui_test.go`: `Model.Update()` — cursor preselects the tier already present in `.env` on entry to `scrTier` (design Wizard flow preselection).
- [x] 6.5 `cmd/somm/setup_test.go`: `t.TempDir()` `.env` round-trip — `saveEnvFile` with all three keys writes and reloads `OPENCODE_API_KEY`, `OPENROUTER_API_KEY`, `SOMM_OC_TIER` (design Threat Matrix mutation note; setup-wizard: Persistence "Save writes the persisted tier").
- [x] 6.6 `cmd/somm/setup_test.go`: without `--force`, `scrTier` is skipped and the value is not overwritten when `.env` already has `SOMM_OC_TIER`; with `--force`, the screen re-shows and may overwrite it (setup-wizard: Tier Capture "not re-asked"/"--force re-asks").
- [x] 6.7 `cmd/somm/main_test.go`: `run()` fails fatal on an unrecognized `SOMM_OC_TIER` value, matching the existing fail-loud style (design Decision 5).

## Phase 7 (PR2b): Integration and Cleanup

- [x] 7.1 Run `go build ./...` and `go vet ./...`; confirm no remaining `profile.Resolve` call site needing `ResolveWithTier` and no signature mismatches.
- [x] 7.2 Run full `go test ./...`; re-confirm `TestGoldenParity` passes byte-for-byte against `testdata/gentle-ai.golden` after all three PRs.
- [x] 7.3 Update developer-facing docs/comments (README, `cmd/somm/main.go` help text) describing tier-aware profile resolution and the `selection`/`frequency` YAML fields.

## Key Learnings

1. `codegraph_explore` returned exact current signatures (`findBestModel`, `mergeDefaults`, `saveEnvFile`, the `screen` iota block) that the design only described narratively, letting tasks cite real insertion points instead of paraphrased ones.
2. The design's "`scrTier` is unconditional in that path" note only holds for fresh/`--force` runs; an existing install upgrading onto this feature has `alreadyConfigured=true` for keys but no persisted tier, so PR2b needs an independent `SOMM_OC_TIER`-presence check rather than reusing `alreadyConfigured` as-is — captured as task 5.7.
3. `Role.Frequency`/`FrequencyWeight` and the frequency enum validation belong to PR2a (currency axis), not PR1, even though both live in the same `role-profiles` spec domain — the design's File Changes table is the authority for the PR boundary, not the spec-domain boundary.
