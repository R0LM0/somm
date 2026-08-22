# Tasks: Derive Plan Quota at Request Time from Live Pricing

## Review Workload Forecast

| Field | Value |
|---|---|
| Estimated changed lines | PR1 ~60, PR2 ~140, PR3 ~650-750, PR4 ~80 (total ~950-1050) |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR1 → PR2 → PR3 → PR4, PR2 strictly before PR3 |
| Delivery strategy | auto-chain |
| Chain strategy | pending |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: pending
400-line budget risk: High

**Forecast note**: design's own estimate (330 lines for PR3) is understated. `plans_test.go`'s
required "all 22 models, two assertions per row" fixture test plus the full 22-model YAML
rewrite alone add ~250-300 lines the design's PR-3 line count did not itemize. PR3 stays
irreducibly atomic (Requests' signature change breaks both `recommend.go` call sites in one
compile unit) but is likely to exceed 400 lines **on its own**, even inside a 4-PR chain — flag
for an explicit maintainer `size:exception` on PR3 specifically, separate from the aggregate
`auto-chain` decision.

### Suggested Work Units

| Unit | Goal | PR | Focused test command | Runtime harness | Rollback boundary |
|---|---|---|---|---|---|
| 1 | Cache-read price plumbing (no ranking change) | PR 1 | `go test ./internal/api/... -run TestEnrichWithOpenRouter -v` | N/A — no network/Bash tool this session (design D2); no consumer yet | Revert `Money.InputCacheRead` + `match.go` copy line |
| 2 | Characterization tests for `resolveQuotaDenominators` | PR 2 | `go test ./internal/api/... -run TestResolveQuotaDenominators -v` | N/A — test-only PR | Revert new test functions only, zero prod risk |
| 3 | `plans` schema + derivation + both call sites | PR 3 | `go test ./internal/profile/plans/... ./internal/api/... -run 'TestCostPerRequest|TestRequests|TestReferenceDataFixture|TestResolveQuotaDenominators|TestFindBestModel_Quota|TestCollectCandidates_Quota' -v` | N/A — no network/Bash tool this session (design D2) | Single-commit revert of `plans.go`, `plans_test.go`, `opencode-zen-go.yaml`, `recommend.go` quota call sites, 5 updated tests |
| 4 | Reason text + spec rewording | PR 4 | `go test ./internal/api/... -run TestBuildReason -v` | N/A — string-formatting only | Revert `buildReason` strings/doc comment + `spec.md` wording |

## Phase 1: Cache-Read Price Plumbing (PR 1)

- [x] 1.1 `internal/api/models.go`: add `InputCacheRead *float64 \`json:"input_cache_read,omitempty"\`` to `Money`.
- [x] 1.2 [RED] `internal/api/match_test.go`: `TestEnrichWithOpenRouter_CopiesInputCacheRead` — present price → parsed float, absent → nil.
- [x] 1.3 [GREEN] `internal/api/match.go` `enrichWithOpenRouter`: parse `match.Pricing.InputCacheRead` via `ParseMoney`, assign when non-nil.
- [x] 1.4 `go test ./internal/api/...` full green; confirm `omitempty` keeps unaffected `EnrichedModel` goldens byte-identical.

## Phase 2: Characterization Tests — RED Net (PR 2, strictly before Phase 3)

- [x] 2.1 `internal/api/recommend_test.go`: direct-call `TestResolveQuotaDenominators_SaturationCapAndHeadroom` (saturating quota==1; scarce `hSat/headroom`; `hasQuota` flag). Closes the direct-unit-test gap the 5 existing `findBestModel`/`buildReason` quota tests don't cover.
- [x] 2.2 `TestResolveQuotaDenominators_UntabulatedFallback` — `hasQuota=false`, `quota==price/pMin`, current `map[string]int` schema.
- [x] 2.3 `TestResolveQuotaDenominators_NilTableAllFallback` and `_EmptySliceNoPanic`.
- [x] 2.4 `go test ./internal/api/... -run TestResolveQuotaDenominators -v` green against pre-rewrite code.

## Phase 3: Plans Schema + Derivation + Call Sites (PR 3 — atomic, largest slice)

- [x] 3.1 `internal/profile/plans/plans.go`: add `Shape`, `ModelPlan`, `Price`, `Window`/consts, `windowDivisor`; `Table.MeasuredAt`→`CuratedAt`, `Models map[string]ModelPlan`.
- [x] 3.2 [RED] `plans_test.go` (new): `TestParseTable` valid / bad-`curated_at` / malformed-YAML.
- [x] 3.3 [GREEN] update `parseTable`/`OpenCodeZenGo` for the renamed field.
- [x] 3.4 [RED] `TestCostPerRequest`: Grok $0.02497, Hy3, DeepSeek off-peak; untabulated/non-positive cost → `ok=false`.
- [x] 3.5 [RED] `TestCostPerRequest_CacheReadFallback` (D5): live ratio, `input*ratio` fallback, explicit `0` honored.
- [x] 3.6 [GREEN] implement `CostPerRequest(ocID, Price) (float64, bool)`.
- [x] 3.7 [RED] `TestRequests_AllWindows`: month==5h×5, month==week×2; Hy3 `multiplier: 8` (spec: budget=$480, quota≈172,058).
- [x] 3.8 [GREEN] implement `Requests(ocID, Price, Window) (float64, bool)`.
- [x] 3.9 [RED] `TestReferenceDataFixture`: 21 curated `reference-data.md` models (22 rows minus table-absent MiniMax M2.5) — exact golden + within-40%-of-published per row, plus explicit table-absent locks for MiniMax M2.5 and Ox Alpha Free.
- [x] 3.10 Rewrite `opencode-zen-go.yaml`: 21 models × shape/cache_read_ratio/tier_usd/multiplier, `curated_at`, off-peak/≤-tier prices (D4), Hy3 `multiplier: 8`; MiniMax M2.5 stays table-absent (D3 — do not inherit M2.7 shape); Ox Alpha Free excluded entirely.
- [x] 3.11 `go test ./internal/profile/plans/... -v` green.
- [x] 3.12 [RED] `internal/api/recommend_test.go`: `TestPriceOf_MapsEnrichedModelPricing`.
- [x] 3.13 [GREEN] implement `priceOf(EnrichedModel) plans.Price` in `recommend.go`.
- [x] 3.14 Add `reqPer5H float64` to `scoredModel`.
- [x] 3.15 [RED] Update the 5 pre-existing quota tests (`TestFindBestModel_QuotaCurrencyRanksHighQuotaAboveLowQuotaWinner`, `TestCollectCandidates_QuotaUntabulatedFallbackUsesPriceOverPMinBridge`, `TestBuildReason_QuotaTabulatedContainsMeasuredAt`, `TestBuildReason_QuotaFallbackOmitsMeasuredAt`, `TestFindBestModel_FrequencyHasNoEffectUnderUSDCurrency`) to `map[string]plans.ModelPlan`, same scenario intent — confirmed non-compiling before 3.16 (also required updating the 4 Phase-2 characterization tests in `recommend_test.go`, which construct `plans.Table` literals directly and would not compile otherwise — undercounted by tasks.md's "5 tests", documented as a deviation below).
- [x] 3.16 [GREEN] rewrite `resolveQuotaDenominators`: call `Requests(ocID, priceOf(m), Window5H)`, store `reqPer5H`; saturation math unchanged.
- [x] 3.17 [GREEN] `buildReason` quota-tabulated branch reads `best.reqPer5H`; wording untouched (Phase 4 scope) except the forced `%d`→`%.0f` type change and `MeasuredAt`→`CuratedAt` field rename.
- [x] 3.18 [RED] `TestCollectCandidates_CuratedNilPriceExcludedNoPanic` — D3 lock.
- [x] 3.19 `go build ./... && go test ./internal/api/... ./internal/profile/plans/... -v` full green.
- [x] 3.20 `go test ./... -run TestRecommendConfig_GoldenParity` — usd-currency path unaffected, PASS.

## Phase 4: Reason Text + Spec Rewording (PR 4)

- [x] 4.1 [RED] rename/update `TestBuildReason_QuotaTabulatedContainsMeasuredAt`→`...CuratedDate`: assert "derivado del precio actual", `CuratedAt`, no "medido".
- [x] 4.2 [RED] rename/update `...OmitsMeasuredAt`→`...OmitsCuratedDate`: assert "sin datos de cuota", no "medido"/"sin cuota medida".
- [x] 4.3 [GREEN] rewrite `buildReason` quota-arm/fallback strings + doc comment per design's exact format; remove all "medido".
- [x] 4.4 Update `openspec/specs/plan-quota-currency/spec.md` (base spec) to the delta's curation-date wording. DEVIATION: skipped — confirmed via `git log -- openspec/specs/` that this repo's convention is main-spec sync at `sdd-archive` time (see the `chore: archive cost-aware-profile-selection` commit, which merged delta specs into `openspec/specs/` — the antecedent feature commit `5116cc6` did not touch `openspec/specs/` at all). The delta spec at `openspec/changes/live-quota-derivation/specs/plan-quota-currency/spec.md` already carries the target end-state wording (curation-date framing, no "measured" claim) and will be merged into the main spec by `sdd-archive`, matching project convention.
- [x] 4.5 `go test ./internal/api/... -run TestBuildReason -v` green.
- [x] 4.6 `go build ./... && go test ./...` full regression across all 4 slices.
