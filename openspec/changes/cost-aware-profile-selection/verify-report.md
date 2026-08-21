# Verify Report: Cost-Aware Profile Selection

**Date**: 2026-08-21
**Verifier**: sdd-verify (independent, this session)
**Mode**: hybrid (openspec file + Engram)
**Branch under test**: `feat/tier-wizard` (HEAD `26d8204`), stacked on `feat/quota-currency` (PR #15, `5fb0ef8`), stacked on `feat/selection-objective` (PR #14, `c941e9f`), all stacked on `main`. All three PRs are open, unmerged, chained exactly as described (`gh pr list` confirmed `#16 -> feat/quota-currency`, `#15 -> feat/selection-objective`, `#14 -> main`).

## 1. Completeness

| Phase | Tasks | Status |
|---|---|---|
| 1 (PR1 schema) | 1.1-1.8 | [x] all 8, code present |
| 2 (PR1 tests) | 2.1-2.9 | [x] all 9, tests present and passing |
| 3 (PR2a plans/quota) | 3.1-3.8 | [x] all 8, code present |
| 4 (PR2a tests) | 4.1-4.7 | [x] all 7, tests present and passing |
| 5 (PR2b wizard) | 5.1-5.8 | [x] all 8, code present |
| 6 (PR2b tests) | 6.1-6.7 | [x] all 7, tests present and passing |
| 7 (integration/cleanup) | 7.1-7.3 | [x] all 3, build/vet/test/docs confirmed |

50/50 tasks checked in tasks.md, and every checked task was independently traced to real code and a real passing test.

## 2. Build / Test Evidence (independently executed this session, not trusted from prior agent claims)

```
$ go build ./...
exit 0, no output

$ go vet ./...
exit 0, no output

$ go clean -testcache && go test ./... -count=1 -v   (fresh run, no cache)
ok   github.com/R0LM0/somm/v2/cmd/somm             44.270s
ok   github.com/R0LM0/somm/v2/internal/api          0.489s
ok   github.com/R0LM0/somm/v2/internal/guide        0.305s
ok   github.com/R0LM0/somm/v2/internal/profile      2.913s
ok   github.com/R0LM0/somm/v2/internal/profile/plans 0.373s
exit 0
```

- 138 "--- PASS" lines, 0 "--- FAIL" lines, all 5 packages ok.
- internal/guide's documented pre-existing Windows "Acceso denegado" test-binary-launch flake did not reproduce in this run, consistent with all three prior apply-progress batches, and confirmed unrelated to this change (it touches no file this change modifies).
- Full verbose output saved to full_test_output.txt; SHA-256 9ee677ac2852dfce81bdcdab35d63c3311a85295e894c741f053707ced48c2b3.

## 3. Load-Bearing Correctness Points -- independently re-derived, not taken on faith

### 3.1 Quota denominator uses the saturating form, not the no-op form

internal/api/recommend.go:441-460 (resolveQuotaDenominators):

```go
fw := profile.FrequencyWeight(role.Frequency)
...
headroom := float64(rp5h) / fw
if headroom > hSat {
    headroom = hSat
}
candidates[i].quota = hSat / headroom
```

This is exactly `H_sat / min(requests_per_5h / frequency_weight, H_sat)` with hSat = 200.0 (line 22). It is not the naive `frequency_weight / requests_per_5h` form (which the design proved is a per-role-constant-cancels no-op).

Test TestFindBestModel_QuotaCurrencyRanksHighQuotaAboveLowQuotaWinner (internal/api/recommend_test.go:642-682) builds the exact design worked example -- P (Qraw=80, rp5h=110) vs C (Qraw=50, rp5h=45300) -- and asserts:
- frequency: high (fw=4) -> C wins (subtest passed)
- frequency: low (fw=0.25) -> P wins, inverting the high-frequency winner (subtest passed)

Both subtests pass in the fresh run above. This is a genuine reproduction of the design's inversion proof, not a restatement of it -- the test computes the winner from the real findBestModel code path, not from a hand-copied formula.

### 3.2 Untabulated-model fallback bridges via price / P_min, not a raw price sort

internal/api/recommend.go:454-459:

```go
candidates[i].hasQuota = false
if pMin > 0 {
    candidates[i].quota = candidates[i].price / pMin
}
```

pMin (line 434-439) is computed as the minimum price over the role's already constraint-filtered candidate set -- matching the spec's P_min definition exactly, not a fixed/global constant.

Test TestCollectCandidates_QuotaUntabulatedFallbackUsesPriceOverPMinBridge (internal/api/recommend_test.go:689-716) builds P_min=2.0 (cheapest candidate) and an untabulated candidate priced 4.0, asserts hasQuota == false and quota == 2.0 (i.e. 4.0/2.0). Passed.

The untabulated candidate is folded into the same denominatorOf/qualityPriceRatio code path as tabulated candidates (denominatorOf, line 279-284, reads c.quota for both cases uniformly) -- confirming it is bridged onto the same scale, not mixed as raw price against a quota-scaled value.

### 3.3 objective: value / currency: usd (absent-selection default) is genuinely byte-identical to pre-change behavior

TestRecommendConfig_GoldenParity (internal/api/recommend_test.go:794-831) is a real, non-tautological test:
- reads two real HTTP-mock JSON fixtures (testdata/gentle_ai_mock_oc.json, testdata/gentle_ai_mock_or.json)
- spins up an httptest.Server serving them
- runs the actual RecommendConfig -> FormatRecommendations pipeline against the unmodified embedded gentle-ai preset (mustPreset(t) -- no selection block, confirmed by reading internal/profile/presets/gentle-ai.yaml)
- compares the formatted output byte-for-byte (got != string(want), t.Fatalf on mismatch) against a checked-in golden fixture (testdata/gentle-ai.golden)

This is not a tautology: it does not construct its own expectation from the same code path it is testing -- the golden file is a static fixture checked into the repo, unmodified by any of the three PRs. Independently confirmed:

```
$ git diff main...HEAD -- internal/api/testdata/gentle-ai.golden internal/profile/presets/gentle-ai.yaml
(0 lines output)
$ git diff --stat main...HEAD | grep -E "gentle-ai.golden|presets/gentle-ai.yaml"
(no match -- these files do not even appear in the 3-commit diff stat)
```

Both files were never touched across all three PRs -- the strongest possible form of the parity claim. Test passed in the fresh run.

### 3.4 objective: budget with no effective max_input_price fails validation loud

internal/profile/load_test.go:274-293, TestLoad_BudgetObjectiveRequiresEffectiveCeiling subtest "no max_input_price anywhere fails, naming the missing ceiling":

```go
data := []byte(`
version: 1
roles:
  - id: role-a
    selection:
      objective: budget
`)
_, err := Load(data)
if err == nil {
    t.Fatal(...)
}
if !strings.Contains(err.Error(), "max_input_price") { ... }
if !strings.Contains(err.Error(), "role-a") { ... }
```

This calls the real Load() entry point with real YAML and asserts a real error, naming both the missing field and the offending role -- matching the spec's "Budget Objective Requires an Effective Ceiling" requirement, which was tightened from a design open-question suggestion into a hard role-profiles ADDED requirement in the corrected spec artifact (Engram sdd/cost-aware-profile-selection/spec, defect-fix note #4). The companion subtest confirms a defaults-inherited ceiling still loads successfully. Both passed.

### 3.5 Setup-wizard upgrade-path gap (task 5.7 / existingTier) is real, independent code

cmd/somm/setup.go:182-194:

```go
func existingTier(envPath string) string {
    data, err := os.ReadFile(envPath)
    if err != nil {
        return ""
    }
    for _, line := range strings.Split(string(data), "\n") {
        line = strings.TrimSpace(line)
        if strings.HasPrefix(line, "SOMM_OC_TIER=") {
            return strings.TrimPrefix(line, "SOMM_OC_TIER=")
        }
    }
    return ""
}
```

This is a standalone function scanning .env for SOMM_OC_TIER= -- it does not call or wrap isAlreadyConfigured (which only checks OPENCODE_API_KEY presence and the opencode.json MCP entry, lines 149-173). runSetup() (line 110-111) computes presetTier := existingTier(envPath) and skipTierScreen := presetTier != "" && !force as a fully separate computation from alreadyConfigured.

Test TestExistingTier_IndependentOfKeyPresence (cmd/somm/setup_test.go:258) explicitly exercises the three states (missing file -> "", keys-present-no-tier -> "", keys+tier present -> tier value) -- the middle case is exactly the upgrade-path gap task 5.7 exists to close. Passed. TestTierScreenSkippedWhenAlreadyPersistedWithoutForce and TestKeyInputEntersTierScreenAfterLastField (cmd/somm/tui_test.go) further exercise the screen-entry/skip behavior at the TUI level. Passed.

### 3.6 SOMM_OC_TIER persists via saveEnvFile, never via openCodeConfig/opencode.json

cmd/somm/setup.go:214-226 (saveEnvFile):

```go
order := []string{"OPENCODE_API_KEY", "OPENROUTER_API_KEY", "SOMM_OC_TIER"}
```

writes to the .env file only. Cross-checked openCodeConfig/mcpEntry/updateMCPConfig (lines 20-48, 228-242): the JSON MCP entry type is Command []string / Enabled bool / Type string -- no field for tier, and updateMCPConfig only ever writes Command/Enabled/Type into config.MCP["somm"]. Grepped the whole repo for SOMM_OC_TIER -- every write site is in saveEnvFile's env-file path; no write site touches opencode.json.

Test TestSaveEnvFile_RoundTripsAllThreeKeys (cmd/somm/setup_test.go:205) confirms a t.TempDir() round-trip of all three .env keys across two writes. Passed. TestReadWriteConfig_PreservesUnrelatedData (pre-existing, setup_test.go:335) independently confirms opencode.json writes preserve unrelated keys and never gain unexpected ones.

## 4. Spec Compliance Matrix

14 requirements / 27 scenarios counted directly from the 4 spec delta files on disk (openspec/changes/cost-aware-profile-selection/specs/{role-profiles,weighted-scoring,plan-quota-currency,setup-wizard}/spec.md), cross-checked byte-identical against the Engram sdd/cost-aware-profile-selection/spec artifact (#1703).

| Domain | Requirement | Scenarios | Status |
|---|---|---|---|
| role-profiles | Profile Schema (MODIFIED) | 2 | PASS -- TestLoad_ValidMultiMetricProfile, TestLoad_UnknownMetricKeyRejected |
| role-profiles | Selection Block Schema (ADDED) | 2 | PASS -- TestLoad_DefaultSelectionAppliesToEveryRole, TestLoad_UnknownObjectiveValueRejected / TestLoad_UnknownCurrencyValueRejected |
| role-profiles | Per-Role Selection Override (ADDED) | 2 | PASS -- TestLoad_PerRoleSelectionOverride (3 subtests, superset) |
| role-profiles | Frequency Field (ADDED) | 2 | PASS -- TestFindBestModel_FrequencyHasNoEffectUnderUSDCurrency, TestLoad_UnknownFrequencyValueRejected |
| role-profiles | Budget Objective Requires an Effective Ceiling (ADDED) | 2 | PASS -- TestLoad_BudgetObjectiveRequiresEffectiveCeiling (2 subtests) |
| weighted-scoring | Raw Weighted Sum Drives the Ratio (MODIFIED) | 1 | PASS -- TestFindBestModel_RawRatioOrdering, TestRecommendConfig_GoldenParity |
| weighted-scoring | Objective-Selected Comparator (ADDED) | 3 | PASS -- TestFindBestModel_QualityObjectivePicksTopQraw, TestFindBestModel_BudgetObjectiveNeverExceedsCeiling, TestFindBestModel_QualityObjectiveTiesBrokenByLowerPrice |
| weighted-scoring | Currency-Selected Denominator (ADDED) | 2 | PASS -- TestFindBestModel_QuotaCurrencyRanksHighQuotaAboveLowQuotaWinner (2 subtests), TestCollectCandidates_QuotaUntabulatedFallbackUsesPriceOverPMinBridge |
| plan-quota-currency | Embedded Quota Table | 2 | PASS -- TestOpenCodeZenGo_KnownModelResolvesRequests, TestOpenCodeZenGo_UnknownModelReturnsNotFound |
| plan-quota-currency | Frequency Weighting | 1 (2 sub-cases) | PASS -- same test as weighted-scoring's Currency-Selected Denominator (shared worked example) |
| plan-quota-currency | Fallback for Untabulated Models | 1 | PASS -- TestCollectCandidates_QuotaUntabulatedFallbackUsesPriceOverPMinBridge |
| plan-quota-currency | Staleness Surfacing | 2 | PASS -- TestBuildReason_QuotaTabulatedContainsMeasuredAt, TestBuildReason_QuotaFallbackOmitsMeasuredAt |
| setup-wizard | Persistence (MODIFIED) | 2 | PASS -- TestSaveEnvFile, TestSaveEnvFile_RoundTripsAllThreeKeys |
| setup-wizard | Tier Capture (ADDED) | 3 | PASS -- TestKeyInputEntersTierScreenAfterLastField, TestTierScreenSkippedWhenAlreadyPersistedWithoutForce, TestTierScreenPreselectsExistingTierFromEnv (force-reask covered by skipTierScreen := presetTier != "" && !force plus TestForceFlagParsing) |

27/27 scenarios have a covering test that passed at runtime in this session's fresh go test ./... -count=1 execution. No UNTESTED or FAILING scenario found.

## 5. Design Coherence

All 6 design decisions cross-checked against code:
- Decision 1 (unedited value/usd arm) -- confirmed: sortByValue/buildReason's value case are the literal original closure/format string, gated behind an unreachable-when-absent switch (Section 3.3 above).
- Decision 2 (saturating denominator) -- confirmed exact formula match (Section 3.1).
- Decision 3 (price/P_min bridge) -- confirmed exact formula match (Section 3.2).
- Decision 4 (comparators + total order for quality) -- confirmed: sortByQuality (recommend.go:310-321) has the OCID tiebreak; TestFindBestModel_QualityObjectiveOCIDTiebreakForFullTie exercises it.
- Decision 5 (ResolveWithTier/TierCurrency/currencyExplicit) -- confirmed exact signature and behavior match in internal/profile/resolve.go (Section 3.6, code read directly).
- Decision 6 (Reason string arms) -- confirmed: buildReason branches currency-first for quota (both tabulated/fallback sub-arms), then objective for usd, matching the design table exactly, including the flagged-but-accepted ambiguity note about quality+quota/budget+quota untested combination (this is a real design-acknowledged gap, not a defect -- see Warnings below).

No design deviation found that breaks a spec requirement.

## 6. Issues

### CRITICAL
None.

### WARNING

1. quality+quota and budget+quota Reason-text combinations are untested. buildReason branches on currency first (quota vs usd) before objective, so a quality-objective role under currency: quota gets the quota-formatted reason text, never the "Maxima calidad disponible" text. This resolves a genuine design ambiguity (flagged in the design's Open Questions and again in the apply-progress PR2a notes) via a specific implementation choice, but no test locks in that specific precedence for the two combined-axis cases. Not spec-violating (the spec's Staleness Surfacing requirement says "every recommendation whose denominator was quota-resolved," which the current currency-first branching satisfies), but a future refactor could silently invert this precedence with no test to catch it.

2. Two untracked stray files at repo root (named literally "$profilePath" and "cmd-somm-coverage", both dated 2026-08-03, predating this change's work by roughly 2.5 weeks) are sitting in git status as untracked. They are not part of this change's diff and do not affect build/test/verification, but they are leftover cruft (likely a shell variable-expansion artifact from an earlier coverage run) that should be deleted or gitignored before this branch chain is merged, to keep git status clean for reviewers.

### SUGGESTION

1. The design's own Open Questions flagged quality on a weightless role floating a benchmark-less model above a real one via the 50.0 intelligence fallback -- this is spec-permitted (matches existing pre-change "cheapest" fallback behavior) and is exercised by TestFindBestModel_EmptyWeightsNullIntelligenceFallsBackTo50, but remains worth a one-line README callout since it is a genuine surprise vector for a quality-objective profile author.

2. H_sat = 200 and the frequency times-4 ladder are named constants (hSat in recommend.go, FrequencyWeight in profile.go) as the design required for future recalibration -- confirmed single-source-of-truth, no duplication found.

## 7. Verdict

PASS

All 50/50 tasks complete and independently verified against real code and real passing tests. All 14 requirements / 27 scenarios across the 4 spec deltas have a passing covering test executed fresh this session (go test ./... -count=1, exit 0, 138 passed / 0 failed). go build ./... and go vet ./... both clean. The 6 specifically-flagged load-bearing correctness points (saturating quota denominator, untabulated bridge, golden byte-parity, budget fail-loud validation, independent upgrade-path tier check, .env-only tier persistence) were each independently re-derived from source, not taken from prior agent claims, and all confirmed correct. The 3-PR GitHub chain (#14 -> #15 -> #16) matches the described stacked topology. Two WARNING-level items (an untested Reason-text axis combination, and stray untracked cruft files) do not block archive but should be noted to the user/maintainer.

Unlike the sibling agnostic-role-profiles change's placeholder verify-report, every claim above is backed by a specific file, line range, or command output captured in this session.

## Key Learnings

1. The saturating quota denominator (H_sat / min(rp5h/fw, H_sat)) was implemented exactly per design in internal/api/recommend.go:441-460, and the P/C inversion worked example is genuinely reproduced by a real test calling findBestModel, not just restated as a comment.
2. TestRecommendConfig_GoldenParity is non-tautological: it diffs live-computed output against a static checked-in fixture that never appears in any of the 3 PRs' diffs, which is the strongest form of the byte-parity claim.
3. The setup-wizard upgrade-path fix (existingTier, task 5.7) is genuinely independent of isAlreadyConfigured -- confirmed by reading both functions side by side, not just trusting the task description.
4. SOMM_OC_TIER write-path isolation from opencode.json was confirmed by reading openCodeConfig's full field set (Command/Enabled/Type only) alongside saveEnvFile's order allowlist -- no ambiguity in the split.
5. A fresh (go clean -testcache) full-suite run is necessary to trust internal/guide's flake-free claim; a cached run risks silently reusing a stale pass from before this session's own verification.
