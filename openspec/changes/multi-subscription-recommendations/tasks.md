# Tasks: Multi-Subscription Recommendations

## Review Workload Forecast

| Field | Value |
|---|---|
| Estimated changed lines | PR1(D) ~200-260, PR2(A) ~450-520, PR3(B) ~420-480, PR4(C) ~520-620, PR5(E) ~460-540 (total ~2,050-2,420) |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR1(D) → PR2(A) → PR3(B) → PR4(C) → PR5(E); PR1 is independent and may run/merge anytime before PR5 |
| Delivery strategy | auto-chain |
| Chain strategy | stacked-to-main |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: High

**Forecast note**: design's own sequencing estimate (~1,270-1,630 total) is a lower bound — the
prior change in this project (`live-quota-derivation`) landed its atomic slice at ~1.25-1.4x the
design number once fixtures/table-driven cases were fully itemized. Applying the same
conservative multiplier here still leaves PR2(A), PR3(B), PR4(C), and PR5(E) each **individually
over the 400-line budget**. PR4(C) and PR5(E) are design-mandated irreducible atomic units
(D6/D10/D11 — see below) and cannot be shrunk further without producing a wrong-price or
silent-empty-result intermediate state: each needs an explicit maintainer `size:exception` on
that specific PR, separate from the aggregate `stacked-to-main` chain decision. PR2(A) and PR3(B)
are large mainly due to the 6 mandatory threat-matrix RED tests (subprocess boundary is new to
this codebase) and should be reviewed as security-relevant even though risk is process isolation,
not code correctness.

### Suggested Work Units

| Unit | Goal | PR | Focused test command | Runtime harness | Rollback boundary |
|---|---|---|---|---|---|
| D | Selection.Providers schema + validation + merge | PR 1 | `go test ./internal/profile/... -run 'TestValidateSelection\|TestLoad\|TestResolveSelection' -v` | N/A — pure schema/YAML, never validated against live host state (D9) | Revert `profile.go`/`load.go` diff; field unused until PR5, zero behavior change |
| A | `discover.go` types, interface, JSON parser | PR 2 | `go test ./internal/api/... -run TestParseDiscoverOutput -v` | N/A — table-driven fixtures only | Delete `discover.go`/`discover_test.go`; interface has zero callers yet |
| B | `execDiscoverer`: subprocess, timeout, cache, single-flight | PR 3 | `go test ./internal/api/... -run TestExecDiscoverer -v -race` | `go test ./internal/api/... -run TestExecDiscoverer_RealCLI -v` (auto-skips via `testing.Short()`/missing `opencode` on PATH) | Revert subprocess additions to `discover.go`+tests; `Client.Discoverer` unwired until PR4 |
| C | `models.go` fields + `api.go` merge + `match.go` D6 guard | PR 4 | `go test ./internal/api/... -run 'TestListModels\|TestMergeDiscovered\|TestEnrichWithOpenRouter' -v` | N/A — fakeDiscoverer-driven; first behavior-changing PR, kill switch is `SOMM_DISCOVERY=off` | Revert `models.go`/`api.go`/`match.go` diff as one commit (atomic, D6) |
| E | `recommend.go` pre-filter, `ProviderStatus`, reasons, formatter | PR 5 | `go test ./internal/api/... -run 'TestCollectCandidates\|TestRecommendConfig\|TestBuildReason\|TestFormatRecommendations' -v` | N/A — fakeDiscoverer-driven | Revert `recommend.go` diff as one commit (atomic, D10/D11); `SOMM_DISCOVERY=off` also neutralizes end-to-end |

## Phase 1: Selection.Providers Schema (PR 1 — no deps, run first/parallel)

- [x] 1.1 `internal/profile/profile.go`: add `Providers []string` to `Selection` (`yaml:"providers,omitempty"`).
- [x] 1.2 `internal/profile/load.go` `validateSelection`: each `Providers` entry non-empty/non-whitespace after trim; malformed entry -> error naming it; explicit `[]` (present-but-empty list) is a load error, distinct from absent/nil (D8).
- [x] 1.3 `internal/profile/load.go` `resolveSelection`: extend role->profile->{value,usd} precedence to `Providers` — nil inherits, non-nil replaces; a role overriding only `objective`/`currency` still inherits `providers` unchanged.
- [x] 1.4 [RED] `load_test.go` `TestValidateSelection_ProvidersEmptyStringEntry` -> error naming the malformed entry.
- [x] 1.5 [RED] `TestLoad_ProvidersExplicitEmptyListIsLoadError` (D8).
- [x] 1.6 [RED] `TestLoad_ProvidersUnconfiguredProviderStillLoads` — `["Anthropic"]` unconfigured still loads (D9, never validated against live host state).
- [x] 1.7 [RED] `TestResolveSelection_RoleOverridesOnlyProviders_InheritsObjectiveAndCurrency`.
- [x] 1.8 [RED] `TestResolveSelection_NoProvidersAnywhere_InheritsEmptyMeansAllConfigured` (spec: Provider Scope Default Is All Configured Providers).
- [x] 1.9 [GREEN] implement until 1.4-1.8 pass; `go test ./internal/profile/... -v`.

## Phase 2: discover.go Types + Parser (PR 2 — no deps)

- [ ] 2.1 `internal/api/discover.go`: define `DiscoveredModel{ProviderID, ID, Name string; InputPerM, OutputPerM float64; CacheReadPerM *float64; ContextLength *int64}` (D7 — same `*PerM` USD-per-1M convention as `plans.Price`).
- [ ] 2.2 `discover.go`: define `ProviderDiscoverer interface { Discover(ctx) ([]DiscoveredModel, error) }`.
- [ ] 2.3 `discover.go`: define raw JSON DTOs for one `opencode models --verbose` record (`id`, `providerID`, `name`, `cost.input`/`cost.output`/`cost.cache_read`) — confirm any remaining field names against a live capture; unknown extra fields must decode-and-ignore (no `json:",string"`/interface{} passthrough).
- [ ] 2.4 `discover.go`: implement `parseDiscoverOutput(raw []byte) ([]DiscoveredModel, error)` — decode JSON array; per-entry drop (not whole-result failure) when `id`/`providerID` missing or `cost` is null (D4).
- [ ] 2.5 [RED] `discover_test.go` `TestParseDiscoverOutput_Valid` — table-driven multi-provider fixture.
- [ ] 2.6 [RED] `TestParseDiscoverOutput_EmptyArray` -> `[]DiscoveredModel{}`, no error.
- [ ] 2.7 [RED] `TestParseDiscoverOutput_MalformedJSON` -> error, non-fatal caller contract.
- [ ] 2.8 [RED] `TestParseDiscoverOutput_MissingIDOrProviderID` -> that entry dropped, siblings retained (D4).
- [ ] 2.9 [RED] `TestParseDiscoverOutput_NullCost` -> that entry dropped, siblings retained.
- [ ] 2.10 [RED] `TestParseDiscoverOutput_UnknownExtraFieldsIgnored` — hostile fixture, PATH-hijack threat-matrix case at the parse layer.
- [ ] 2.11 [GREEN] implement until 2.5-2.10 pass; `go test ./internal/api/... -run TestParseDiscoverOutput -v`.

## Phase 3: execDiscoverer Subprocess Boundary (PR 3 — depends on Phase 2/A)

- [ ] 3.1 `discover.go`: `execDiscoverer` implementing `ProviderDiscoverer` — `exec.LookPath("opencode")` only, `exec.CommandContext(ctx, path, "models", "--verbose")`, fixed argv, never a shell.
- [ ] 3.2 `discover.go`: 5s timeout via `context.WithTimeout`; process killed and reaped on deadline.
- [ ] 3.3 `discover.go`: `io.LimitReader` on stdout at 8 MiB; over-limit treated as malformed, warn+continue.
- [ ] 3.4 `discover.go`: stderr captured, truncated to 512 bytes before logging on non-zero exit.
- [ ] 3.5 `discover.go`: in-process TTL cache (success 5m, failure 60s) + single-flight so concurrent `Discover` calls share one process.
- [ ] 3.6 `discover.go`: `SOMM_DISCOVERY=off` short-circuits to disabled/no-op (rollback path); `SOMM_OC_DISCOVERY_REFRESH=1` appends `--refresh` to argv (D1 escape hatch).
- [ ] 3.7 [RED] `TestExecDiscoverer_ArgvExactNoInterpolation` — threat matrix: binary resolution.
- [ ] 3.8 [RED] `TestExecDiscoverer_HostileOutputIgnoresUnknownFields` — threat matrix: PATH hijack.
- [ ] 3.9 [RED] `TestExecDiscoverer_OversizeStdoutTreatedAsMalformed` — threat matrix: unbounded stdout.
- [ ] 3.10 [RED] `TestExecDiscoverer_HangPastDeadlineKilled` — threat matrix: hang/no exit (injected `runner func(ctx) ([]byte, error)` respecting cancellation).
- [ ] 3.11 [RED] `TestExecDiscoverer_NonZeroExitStderrTruncated` — threat matrix: non-zero exit.
- [ ] 3.12 [RED] `TestExecDiscoverer_ConcurrentCallsSingleFlight` — threat matrix: concurrent tool calls, counting runner asserts exactly one process.
- [ ] 3.13 [RED] `TestExecDiscoverer_TTLCache` — success cached 5m, failure cached 60s.
- [ ] 3.14 [RED] `TestExecDiscoverer_DisabledByEnv` — `SOMM_DISCOVERY=off` never invokes runner.
- [ ] 3.15 [GREEN] implement until 3.7-3.14 pass; `go test ./internal/api/... -run TestExecDiscoverer -v -race`.
- [ ] 3.16 `cmd/somm/main.go`: document `SOMM_DISCOVERY`/`SOMM_OC_DISCOVERY_REFRESH` in relevant tool/help text.
- [ ] 3.17 [Integration, skippable] `TestExecDiscoverer_RealCLI` — skip via `testing.Short()` and `exec.LookPath("opencode")` failure; assert sane $/M range (D7 regression guard).

## Phase 4: Merge + Price-Source Guard (PR 4 — depends on Phase 2/A; ATOMIC, D6)

- [ ] 4.1 `internal/api/models.go`: add `ProviderID`, `ProviderName`, `ModelSlug`, `PriceSource string` (all `omitempty`) to `EnrichedModel`; add `Discoverer ProviderDiscoverer` to `Client` (nil = default `execDiscoverer`).
- [ ] 4.2 `internal/api/api.go` `ListModels`: add Discoverer call to the existing `WaitGroup` alongside `fetchOC(go)`/`fetchOC(zen)` (max, not sum); error -> `slog.Warn`, continue (D4, mirrors `fetchOpenRouter` warn arm).
- [ ] 4.3 `api.go`: implement `mergeKey(providerID, slug string) string = providerID + "/" + slug` (D5).
- [ ] 4.4 `api.go`: implement `mergeDiscovered` — normalize CLI providers `opencode`/`opencode-go`/`opencode-zen` -> `"opencode"`; `OCID` stays the bare slug only when `ProviderID=="opencode"`, else namespaced `providerID/slug`; `ModelSlug` always the bare slug; convert `cost.*`/1e6 into `Pricing` (`Money`), set `PriceSource: "opencode-cli"` (D6, D7).
- [ ] 4.5 `internal/api/match.go` `enrichWithOpenRouter`: skip the `Pricing` overwrite when `model.PriceSource != ""`, but still copy `Benchmarks`/`ContextLength`/`Reasoning` (D6 — the exact guard this slice cannot ship without).
- [ ] 4.6 `match.go`/`MatchOR` callers: match CLI-sourced models by `ModelSlug` (bare), not the namespaced `OCID`, so OpenRouter alias matching still resolves.
- [ ] 4.7 [RED] `api_test.go` `TestListModels_MergesDiscoveredModels` — fakeDiscoverer models merge alongside OC Go/Zen.
- [ ] 4.8 [RED] `TestListModels_DiscoveryFailureDegradesGracefully` — fakeDiscoverer error -> output unchanged.
- [ ] 4.9 [RED] `TestListModels_ZeroDiscoveredProvidersByteIdentical` — empty discovery -> pre-change golden.
- [ ] 4.10 [RED] `TestMergeDiscovered_DedupeKeyIsProviderIDPlusSlug` — same slug, different providerID stay distinct (`openai/gpt-5.6` vs OC Go's `gpt-5.6`).
- [ ] 4.11 [RED] `match_test.go` `TestEnrichWithOpenRouter_SkipsPricingWhenPriceSourceSet` — CLI price untouched, benchmarks/context/reasoning still copied (D6 — MUST land with 4.5 in the same commit).
- [ ] 4.12 [RED] `TestEnrichWithOpenRouter_StillOverwritesPricingWhenNoPriceSource` — pre-existing OC/OR-only path regression lock.
- [ ] 4.13 [GREEN] implement until 4.7-4.12 pass; `go test ./internal/api/... -run 'TestListModels|TestMergeDiscovered|TestEnrichWithOpenRouter' -v`.
- [ ] 4.14 `go build ./...` — confirm `api.go` + `match.go` land in one commit (merge without the guard is a wrong price, not an incomplete feature).

## Phase 5: Pre-Filter + Reasons + Formatter (PR 5 — depends on Phase 1/D + Phase 4/C; ATOMIC, D10/D11)

- [ ] 5.1 `internal/api/recommend.go` `collectCandidates` step (3): add provider-scope hard constraint — when `role.Selection.Providers` non-empty, candidate `ProviderID` (case-insensitive) must be in that set, else skip (D10 — same tier as `MinContext`/`MaxInputPrice`, applies on both max-2/max-3 passes).
- [ ] 5.2 `recommend.go`: extend `ProviderStatus` with `Ranked bool` + `ExcludedReason string`; `RecommendConfig` emits one entry per discovered provider (D12).
- [ ] 5.3 `recommend.go`: zero-price CLI models (`cost.input==0 && cost.output==0`) excluded via the existing nil/zero-price gate, annotated with the flat-rate `ExcludedReason` ("flat-rate subscription, no usage-cap ranking available"), distinguishable from the generic no-pricing reason.
- [ ] 5.4 `recommend.go`: when a role's `Providers` scope filters out every candidate, resolve via the existing nil-`findBestModel` path -> empty-`Model` `Recommendation` with a scope-specific `Reason` (D11 — MUST land with 5.1 in the same commit, a filter without the reason is a silent empty result).
- [ ] 5.5 `recommend.go` `buildReason`: fix `subInfo`'s `" (Zen)"` fallback-for-anything-not-go/both bug — emit the actual provider name for non-OC providers.
- [ ] 5.6 `recommend.go`: add `slugOf`/`familyOf` helpers for the pre-filter and `ExcludeFamilyOf` logic against namespaced CLI `OCID` values.
- [ ] 5.7 `recommend.go` `FormatRecommendations`: render `ProviderStatus.Ranked`/`ExcludedReason` (stated once per provider).
- [ ] 5.8 [RED] `recommend_test.go` `TestCollectCandidates_ProviderScopeFiltersToConfiguredSet` — scoped to `["OpenCode Go"]`, 3-provider candidate pool -> only OpenCode Go remains.
- [ ] 5.9 [RED] `TestCollectCandidates_ProviderScopeSurvivesCapRelaxation` — scope applies unchanged across max-2->max-3.
- [ ] 5.10 [RED] `TestRecommendConfig_NoMatchingProviderResolvesToFallbackReason` — scoped to `["Anthropic"]` unconfigured -> empty-`Model`, scope-specific `Reason`, no panic, other roles unaffected (D11 atomicity case).
- [ ] 5.11 [RED] `TestRecommendConfig_ZeroPriceModelNeverWinsSurfacesFlatRateReason` — only $0 candidates -> fallback wins, `ProviderStatus.ExcludedReason` carries the flat-rate text.
- [ ] 5.12 [RED] `TestBuildReason_NonOCProviderSubInfoUsesProviderName` — regression lock for the `subInfo` fix.
- [ ] 5.13 [RED] `TestFormatRecommendations_RendersProviderExclusionReason`.
- [ ] 5.14 [GREEN] implement until 5.8-5.13 pass; `go test ./internal/api/... -run 'TestCollectCandidates|TestRecommendConfig|TestBuildReason|TestFormatRecommendations' -v`.
- [ ] 5.15 `go build ./... && go test ./...` — full regression; confirm byte-identical output with zero discovered providers (Behavior Unchanged golden).
