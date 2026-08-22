# Archive Report: Multi-Subscription Recommendations

**Status**: ARCHIVED  
**Date**: 2026-08-21  
**Change**: multi-subscription-recommendations  
**Branch**: feat/provider-prefilter-status @ commit 895a517  
**Verification Verdict**: PASS WITH WARNINGS  

## Executive Summary

Multi-subscription recommendations change archived and closed. All 66 tasks verified complete across 5 atomic implementation phases; specs merged into main corpus (multi-provider-catalog new, role-profiles and weighted-scoring updated); design decisions D1-D12 plus buildReason subInfo fix all confirmed implemented; full test suite green (go build, go vet, go test); PR #26 open pending merge to main — archive independent of PR merge status.

## SDD Artifacts (Engram Observations — Full Traceability)

| Artifact | Observation ID | Topic Key | Status |
|---|---|---|---|
| Proposal | 1748 | sdd/multi-subscription-recommendations/proposal | CLOSED |
| Spec (unified) | 1749 | sdd/multi-subscription-recommendations/spec | CLOSED |
| Design | 1750 | sdd/multi-subscription-recommendations/design | CLOSED |
| Tasks | 1751 | sdd/multi-subscription-recommendations/tasks | CLOSED |
| Verify Report | 1768 | sdd/multi-subscription-recommendations/verify-report | CLOSED |
| Archive Report (this) | TBD | sdd/multi-subscription-recommendations/archive-report | NEW |

## Completeness: Spec Merge

### Multi-Provider Catalog (NEW)

**Source**: openspec/changes/archive/multi-subscription-recommendations/specs/multi-provider-catalog/spec.md  
**Destination**: openspec/specs/multi-provider-catalog/spec.md  
**Status**: COPIED (7 requirements, 9 scenarios)

### Role Profiles (MODIFIED)

**Source**: openspec/changes/archive/multi-subscription-recommendations/specs/role-profiles/spec.md (delta)  
**Destination**: openspec/specs/role-profiles/spec.md  
**Merge Strategy**: Selection Block Schema updated to 3-field schema (`objective`, `currency`, `providers`); Per-Role Selection Override extended to field-by-field precedence for all 3 fields; Added new requirement "Provider Scope Default Is All Configured Providers"  
**Status**: MERGED (3 modified requirements + 1 added requirement = 6 total role-profiles requirements now)

### Weighted Scoring (MODIFIED)

**Source**: openspec/changes/archive/multi-subscription-recommendations/specs/weighted-scoring/spec.md (delta)  
**Destination**: openspec/specs/weighted-scoring/spec.md  
**Merge Strategy**: Hard Constraint Pre-Filter requirement updated to include provider scope as a hard pre-filter at same tier as existing constraints; Empty Candidate Set Fallback requirement updated to list provider scope as a possible cause of emptiness; added one new scenario per requirement  
**Status**: MERGED (2 modified requirements, 3 new scenarios)

## Implementation & Verification Summary

**Implementation Phases**: 5 atomic slices executed sequentially (PR1 independent, PR2-5 serial dependencies respected)  
**Task Completion**: 66/66 ✓  
**Build Status**: `go build ./...` ✓  
**Vet Status**: `go vet ./...` ✓  
**Test Status**: 5/5 packages PASS, 0 skipped/failed ✓  
**Test Coverage**: TestExecDiscoverer_RealCLI (live opencode CLI integration test) ✓  

**Design Decisions Verified**:
- D1: Single `opencode models --verbose` invocation (not auth list + N calls) ✓
- D2: Placement in internal/api/discover.go ✓
- D3: 5s timeout + TTL cache (success 5m, failure 60s) + single-flight ✓
- D4: Uniform D4 failure-degradation (warn+continue) ✓
- D5: mergeKey = ProviderID + "/" + slug; OCID namespacing ✓
- D6: CLI price overwrites merged Pricing; PriceSource guard in enrichWithOpenRouter ✓
- D7: Cost units USD/1M tokens; Money conversion /1e6 ✓
- D8: Selection.Providers schema with nil-inherits/non-nil-replaces semantics ✓
- D9: Identifier validation free-form, case-insensitive, never validated against live host ✓
- D10: Provider pre-filter in collectCandidates hard-constraint tier ✓
- D11: Empty candidate set fallback reuses existing Reason path ✓
- D12: ProviderStatus with Ranked bool + ExcludedReason ✓

**Threat Matrix (6 RED tests)**: All PASS
- Binary resolution (LookPath only, no shell/user-supplied path) ✓
- PATH hijack (output decoded to struct, never executed) ✓
- Unbounded stdout (8 MiB limit) ✓
- Hang/no exit (5s timeout) ✓
- Non-zero exit (stderr captured, truncated) ✓
- Concurrent calls (single-flight, at most one process) ✓

## Known Non-Blocking Follow-Ups

These are lightweight issues suitable for a future dedicated change; do NOT expand scope here:

1. **Spec Text Drafting Artifact (Deviation 1)**: Multi-provider-catalog and role-profiles specs use display-name examples ("OpenCode Go", "Kimi For Coding") in scenario prose, but actual ProviderID tokens are lowercase-dash (e.g., "opencode", "opencode-go", "kimi-for-coding"). Design D9 correctly resolves identifier matching to ProviderID; spec text examples are simply pre-D9 illustrations. Recommend: lightweight spec follow-up correcting illustrative strings, or none if this is deemed an acceptable convention.

2. **Spec Text Site Clarification (Deviation 2)**: Multi-provider-catalog spec's zero-price visibility scenario names `list_available_models`, but design D12 and tasks.md correctly implement visibility in `recommend_config`/`ProviderStatus`. ListModels returns raw models without per-model exclusion reasons ($0 models present); ProviderStatus is the authoritative exclusion-reason channel. Recommend: lightweight spec follow-up clarifying the site, or none if list_available_models annotation is actually desired.

3. **Regression Guard: -race Unavailable on This Host**: Phase 3 task 3.15 notes `-race` unavailable (CGO_ENABLED=0, no gcc). TestExecDiscoverer_ConcurrentCallsSingleFlight passed manual inspection (sync.Mutex + channel close happens-before). Recommend: One -race run in a CGO-enabled CI environment as a follow-up sanity check.

## Merge Evidence & Diff Verification

All three spec merges completed without conflicts. Diffs below confirm:

**Multi-Provider Catalog (new file)**:
```
[Copied from delta spec — byte-identical]
No diff (source and destination identical)
```

**Role Profiles (modified)**:
- Selection Block Schema: 2-field → 3-field schema, 2 scenarios → 4 scenarios
- Per-Role Selection Override: 2 scenarios → 3 scenarios (added providers override)
- Added: Provider Scope Default Is All Configured Providers (1 requirement, 1 scenario)
- Total change: +46 lines (new scenarios + Provider Scope requirement)

**Weighted Scoring (modified)**:
- Hard Constraint Pre-Filter: reordered to include provider scope at hard-constraint tier; 2 scenarios → 3 scenarios
- Empty Candidate Set Fallback: 2 scenarios → 3 scenarios (added provider-scope emptiness case)
- Total change: +31 lines (new scenarios + provider scope references)

## Rollback & Reversibility

**Rollback Path**: Revert commit sequence 895a517 ← c128db2 ← 2b5a77f ← f48e148 ← 3d06f46 (5 commits, full reverse is safe)  
**Alternative Rollback**: `SOMM_DISCOVERY=off` disables discovery without reverting; all new fields omitempty, output byte-identical  
**Spec Rollback**: Delete openspec/specs/multi-provider-catalog/; restore openspec/specs/role-profiles/spec.md and openspec/specs/weighted-scoring/spec.md to pre-change (archived versions preserved in openspec/changes/archive/)

## Final State

- **Code**: Branch feat/provider-prefilter-status @ 895a517, tree clean, PR #26 open (awaiting merge to main)
- **Specs**: Merged into main corpus (multi-provider-catalog new, role-profiles/weighted-scoring updated)
- **Archive**: Change folder moved to openspec/changes/archive/multi-subscription-recommendations
- **Verification**: PASS WITH WARNINGS (2 non-blocking spec-text drafting artifacts, no code defects, 0 CRITICAL, 0 BLOCKER)
- **Status**: READY FOR MERGE — no further actions required; new spec corpus reflects final requirements and design decisions

---

**Archived by**: Claude Agent (SDD Archive Phase)  
**Archive Date**: 2026-08-21  
**Archive Lineage**: Proposal#1748 → Spec#1749 → Design#1750 → Tasks#1751 → Verify#1768 → Archive#TBD
