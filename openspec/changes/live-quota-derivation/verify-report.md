```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:ba655085ea99f3260beb43c09ffca4bd7318cfeb5932f1658c67e79be5db3814
verdict: pass
blockers: 0
critical_findings: 0
requirements: 4/4
scenarios: 9/9
test_command: go test ./... -count=1
test_exit_code: 0
test_output_hash: sha256:3fa9fb2e3e19c7235ad90c4ef1efb88b1539b76b015c2db47a4b0f7787eef1c3
build_command: go build ./...
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report

**Change**: live-quota-derivation
**Version**: delta spec, corrected 2026-08-21 after prior FAIL
**Mode**: Standard

**Re-verify context**: re-run after a prior FAIL (2 CRITICAL, both spec-text/implementation desyncs, no code defects). The orchestrator edited two scenario blocks in the delta spec.md to match the already-shipped, already-tested code (design D3 exclusion-at-collectCandidates; design D5 cache_read_ratio fallback). No production code changed since the prior verify pass. This pass independently re-derived both fixed scenarios against actual code and re-ran the full suite.

### Completeness

| Metric | Value |
|--------|-------|
| Tasks total | 34 |
| Tasks complete | 34 |
| Tasks incomplete | 0 |

Unchanged from prior verify pass; only the two spec.md scenario blocks and the Notes for Design footer changed.

### Build & Tests Execution

**Build**: Passed
```
$ go build ./...
(no output, exit 0)
```

**Tests**: All passed (full repo, uncached)
```
$ go test ./... -count=1
ok  	github.com/R0LM0/somm/v2/cmd/somm	45.261s
ok  	github.com/R0LM0/somm/v2/internal/api	0.602s
ok  	github.com/R0LM0/somm/v2/internal/guide	0.356s
ok  	github.com/R0LM0/somm/v2/internal/profile	3.043s
ok  	github.com/R0LM0/somm/v2/internal/profile/plans	2.536s
```
go vet ./... also clean (exit 0, no output).

**Coverage**: Not available (no coverage tooling configured in this repo; not required by design's Testing Strategy)

### Spec Compliance Matrix

| Requirement | Scenario | Test | Result |
|---|---|---|---|
| Embedded Quota Table | Table resolves a known model shape and tier (multiplier default 1) | TestParseTable, TestReferenceDataFixture/grok-4.5, TestRequests_AllWindows | COMPLIANT |
| Embedded Quota Table | Table omits an unknown model id | TestCostPerRequest/untabulated_ocID, TestReferenceDataFixture/minimax-m2.5, .../ox-alpha-free | COMPLIANT |
| Embedded Quota Table | Multiplier scales window budget - Hy3, 8x | TestRequests_AllWindows/Hy3_multiplier_8x | COMPLIANT |
| Fallback for Untabulated Models | Untabulated model still receives a comparable rank | TestCollectCandidates_QuotaUntabulatedFallbackUsesPriceOverPMinBridge, TestResolveQuotaDenominators_UntabulatedFallback | COMPLIANT |
| Fallback for Untabulated Models | Curated model with no resolvable live price is excluded from candidacy | TestCollectCandidates_CuratedNilPriceExcludedNoPanic | COMPLIANT - spec text corrected to match D3 exclusion-at-collectCandidates resolution; verified internal/api/recommend.go line 357 excludes before quota resolution runs, test asserts model absent from candidates, no panic |
| Staleness Surfacing | Quota-ranked reason carries curation date, not a measurement claim | TestBuildReason_QuotaTabulatedContainsCuratedDate | COMPLIANT |
| Staleness Surfacing | Price-fallback reason omits quota staleness claim | TestBuildReason_QuotaFallbackOmitsCuratedDate | COMPLIANT |
| Cache-Read Price Included in Cost Calculation | Cache-read price contributes to cost - Grok 4.5 | TestCostPerRequest/Grok_4.5, TestReferenceDataFixture/grok-4.5 | COMPLIANT |
| Cache-Read Price Included in Cost Calculation | Missing live cache-read price falls back to curated ratio | TestCostPerRequest_CacheReadFallback/nil_cache-read_price | COMPLIANT - spec text corrected to match D5 ratio-fallback resolution; verified internal/profile/plans/plans.go lines 116-131 computes cacheReadPerM as CacheReadRatio times InputPerM when CacheReadPerM is nil, test asserts non-zero ratio-fallback result 0.0004, distinct from explicit-zero case 0.0002 |

**Compliance summary**: 9/9 scenarios compliant

### Correctness (Static Evidence)

| Requirement / Mechanism | Status | Notes |
|---|---|---|
| plans.Table/ModelPlan/Price/Window schema (D1) | Implemented | plans.go:29-83; windowDivisor exact 5/2/1 Go constants, no YAML |
| CostPerRequest / Requests derivation | Implemented | Golden values (Grok 0.02497, Hy3 0.0027898, DeepSeek 0.0028732) and reference-data.md's 21 published rows |
| plans never imports api (seam via Price) | Implemented | No internal/api import in plans.go; Price struct is the sole cross-package contract |
| priceOf(EnrichedModel) plans.Price | Implemented | recommend.go:430-443, nil-safe, scales to per-1M |
| scoredModel.reqPer5H caching | Implemented, wired end-to-end | Set once in resolveQuotaDenominators, consumed directly by buildReason, no re-derivation |
| D2 - OpenRouter price proxy | Accepted risk, documented | priceOf is the single swap-point; buildReason never claims measured |
| D3 - curated-but-unpriced model excluded from candidacy, no panic | Implemented, tested, and now spec-aligned | recommend.go:357 filters before resolveQuotaDenominators runs; TestCollectCandidates_CuratedNilPriceExcludedNoPanic locks it; spec.md's Fallback for Untabulated Models scenario now describes exclusion, matching shipped behavior |
| D4 - cheaper variant only, no detection | Implemented | All 21 YAML rows use off-peak/tier-limited prices |
| D5 - cache_read_ratio fallback | Implemented, tested, and now spec-aligned | plans.go:116-131; spec.md's Missing live cache-read price scenario now describes the ratio fallback, matching shipped behavior |
| medido removed from production output | Confirmed | Grep across internal/: only in unrelated doc and negative test assertions |
| EnrichedModel.Pricing.InputCacheRead plumbing | Implemented | models.go:17,71, match.go:92-94, omitempty keeps unaffected goldens byte-identical |
| Task 4.4 deviation (main openspec/specs/ sync deferred to archive) | Confirmed correct per repo convention | Unchanged from prior pass |

### Coherence (Design)

| Decision | Followed? | Notes |
|---|---|---|
| D1 - window divisors 5/2/1 | Yes | Exact Go map constants, hSat=200 untouched |
| D2 - OpenRouter price proxy accepted risk | Yes | Documented, isolated in priceOf |
| D3 - Muse Spark filtered upstream | Yes, code/tests/spec text now agree | Prior verify CRITICAL-1 gap is fixed; spec.md scenario now describes exclusion, matching design and shipped code |
| D4 - always cheaper variant | Yes | Verified against reference table |
| D5 - cache_read_ratio fallback | Yes, code/tests/spec text now agree | Prior verify CRITICAL-2 gap is fixed; spec.md scenario now describes the ratio fallback, matching design and shipped code |
| Requests returns float64 not int | Yes | plans.go:143 |
| buildReason exact format string incl. tilde prefix | Yes | recommend.go:652, byte-for-byte match to design's specified format |

### Disturbance Check

Diffed the corrected spec.md against the prior verify pass's retrieved content (Engram id 1742 pre-correction record and the previous verify-report.md's quoted scenario text):
- Embedded Quota Table requirement and its 3 scenarios: unchanged verbatim.
- Staleness Surfacing requirement and its 2 scenarios: unchanged verbatim.
- Cache-Read Price Included in Cost Calculation scenario Cache-read price contributes to cost - Grok 4.5: unchanged verbatim.
- Only the Curated model with no resolvable live price scenario and the Missing live cache-read price falls back to curated ratio scenario were edited, plus the Notes for Design footer heading changed from unresolved to resolved (its body already described the D1-D5 resolutions in both versions, so this is a heading-only correction).
No other requirement, scenario, or task text was disturbed by the fix.

### Issues Found

**CRITICAL**: None.

**WARNING**: None.

**SUGGESTION**:
1. D2's un-run OpenRouter raw-body-inspection spike remains an accepted, documented follow-up per design - no action needed for this change, worth tracking as a standalone future task if not already.
2. EnrichedModel JSON golden byte-identity for InputCacheRead's omitempty behavior was not independently re-verified in this pass beyond no test failures observed - a dedicated golden-diff spot check would close this out fully, though risk is low given TestRecommendConfig_GoldenParity passed.
3. Commit-splitting the accumulated uncommitted working-tree changes into the 4 planned PR-chain commits, and PR3's already-flagged over-budget line count (951 vs 800-line cap), remain outstanding delivery-phase items per apply-progress - not verify blockers, but should be resolved before/during archive delivery.

### Verdict

**PASS**

Both CRITICAL findings from the prior verify pass are resolved. The fix was textual only: two scenario blocks in the delta spec were rewritten to match design D3's and D5's already-resolved, already-shipped, already-tested behavior. This pass independently re-derived both corrected scenarios against the actual code and their locking tests, confirmed no other requirement/scenario/task was disturbed by the edit, and re-ran the full build/vet/test suite clean. All 4 requirements (9/9 scenarios) are compliant, all 34 tasks remain complete, and no code changed since the prior pass. Ready for sdd-archive.
