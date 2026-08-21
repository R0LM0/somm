# Archive Report: Cost-Aware Profile Selection

**Change**: `cost-aware-profile-selection`  
**Archived To**: `openspec/changes/archive/2026-08-21-cost-aware-profile-selection/`  
**Archive Date**: 2026-08-21  
**Status**: **ARCHIVED**

## Executive Summary

Successfully archived the `cost-aware-profile-selection` SDD change after completion of all three chained PRs (#14, #17, #16), verification (PASS, 0 CRITICAL, 2 non-blocking WARNINGs), and merger into main at v2.4.0. All delta specs have been merged into `openspec/specs/` (role-profiles, weighted-scoring, plan-quota-currency, setup-wizard), completing the cost-aware ranking engine with three objective modes (value/quality/budget), quota-currency support, and tier-aware configuration.

## Artifacts Archived

| Artifact | Status | Location |
|----------|--------|----------|
| proposal.md | ✓ Archived | `archive/2026-08-21-cost-aware-profile-selection/proposal.md` |
| specs/ | ✓ Merged into openspec/specs/ | role-profiles, weighted-scoring, plan-quota-currency, setup-wizard |
| design.md | ✓ Archived | `archive/2026-08-21-cost-aware-profile-selection/design.md` |
| tasks.md | ✓ Archived (50/50 tasks complete) | `archive/2026-08-21-cost-aware-profile-selection/tasks.md` |
| verify-report.md | ✓ Archived (PASS) | `archive/2026-08-21-cost-aware-profile-selection/verify-report.md` |

## Specs Synced to Main Specifications

### role-profiles (`openspec/specs/role-profiles/spec.md`)
- **Action**: Created (merged sibling `agnostic-role-profiles` base + this change's deltas)
- **Changes**: Modified "Profile Schema" to include selection block and frequency field; added 4 new requirements
  - Requirement: Selection Block Schema (profile + role-level)
  - Requirement: Per-Role Selection Override
  - Requirement: Frequency Field
  - Requirement: Budget Objective Requires an Effective Ceiling

### weighted-scoring (`openspec/specs/weighted-scoring/spec.md`)
- **Action**: Created (merged sibling `agnostic-role-profiles` base + this change's deltas)
- **Changes**: Modified "Raw Weighted Sum..." requirement to add currency/objective selection; added 2 new requirements
  - Requirement: Objective-Selected Comparator (value/quality/budget)
  - Requirement: Currency-Selected Denominator (usd/quota)

### plan-quota-currency (`openspec/specs/plan-quota-currency/spec.md`)
- **Action**: Created (NEW spec, entirely from this change)
- **Contents**: 4 requirements defining quota table, frequency weighting, untabulated fallback, staleness surfacing

### setup-wizard (`openspec/specs/setup-wizard/spec.md`)
- **Action**: Updated (merged existing spec + this change's delta)
- **Changes**: Modified "Persistence" requirement; added "Tier Capture" requirement
  - Requirement: Tier Capture (new blocking screen, persists SOMM_OC_TIER)

## Final-State Authority Chain

This archive report applies the Final-State Authority hierarchy per the SDD skill:

1. **Native Review Authority**: All 3 chained PRs (#14, #17, #16) merged into main (github.com/R0LM0/somm), tagged v2.4.0 with live GitHub Releases binaries. Git status confirmed via `git log --oneline` and `git tag -l v2.4.0`.

2. **Persisted Tasks Artifact**: All 50 implementation tasks (Phases 1-7, PR1/PR2a/PR2b combined) are marked `[x]` complete in the archived `tasks.md`. Verified in Engram observation #1705 at 2026-08-20 20:50:26; sdd-apply re-verification confirmed at phase completion per the apply-progress record.

3. **Verification Report**: `sdd-verify` returned PASS (0 CRITICAL, 2 non-blocking WARNINGs) at 2026-08-21 03:16:24 (Engram observation #1712). The two warnings concern:
   - (a) quality+quota/budget+quota Reason-text combined-axis precedence is untested (currency-first branching, real design-acknowledged ambiguity resolved by implementation choice)
   - (b) two untracked stray files at repo root ($profilePath, cmd-somm-coverage, dated 2026-08-03, pre-existing and unrelated to this change)

4. **Explicit Final-State Facts from Launch Prompt**: This archive was launched after the user confirmed all 3 PRs merged, released as v2.4.0, and `sdd-verify` PASS. No blocking facts contradicted the archive.

## Sequence: Handling Unarchived Sibling Dependency

Per the proposal and spec, this change's deltas for `role-profiles` and `weighted-scoring` were written against the sibling `agnostic-role-profiles` change's delta files (still unarchived in `openspec/changes/agnostic-role-profiles/specs/`).

**Action Taken**: 
- Retrieved the sibling's delta spec files as the base (containing the original capabilities already shipped on main)
- Layered this change's MODIFIED/ADDED blocks on top
- Created complete, merged specs in `openspec/specs/{role-profiles,weighted-scoring}/spec.md`
- Did NOT archive the sibling change (explicitly deferred per the launch instructions and the proposal)

**Result**: The merged specs in `openspec/specs/` now contain:
1. All capabilities from the sibling's already-shipped deltas (Profile Schema, Capability Token Support, Defaults Merge, Resolution Order, Fail-Loud Validation, Hard Constraint Pre-Filter, Multi-Metric Normalization, Empty Candidate Set Fallback, Assignment Spreading)
2. All modifications and additions from this change (Selection Block Schema, Per-Role Selection Override, Frequency Field, Budget Objective Requires Ceiling, Objective-Selected Comparator, Currency-Selected Denominator)

This prevents duplication of unarchived delta sets stacking on the same two specs — the sibling's capability definitions are now properly merged into base, and the change is cleanly closed.

## Observations Recorded for Traceability

All SDD artifacts retrieved from Engram (hybrid mode) with observation IDs for audit trail:

| Artifact | Engram ID | Retrieved |
|----------|-----------|-----------|
| Proposal | #1702 | 2026-08-20 20:18:18 |
| Spec (deltas) | #1703 | 2026-08-20 20:35:04 |
| Design | #1704 | 2026-08-20 20:39:32 |
| Tasks | #1705 | 2026-08-20 20:50:26 |
| Verify-Report | #1712 | 2026-08-21 03:16:24 |

No review transactions/receipts required: `reviewGate` is structurally absent (no RDD receipt-driven development conducted for this candidate; ordinary repository policy per project convention).

## Verification Checklist

- [x] Main specs updated correctly (role-profiles, weighted-scoring, plan-quota-currency, setup-wizard)
- [x] Change folder moved to archive (2026-08-21-cost-aware-profile-selection)
- [x] Archive contains all artifacts (proposal, specs, design, tasks, verify-report)
- [x] Archived tasks.md has all 50 implementation tasks checked (no unchecked stale items)
- [x] Active changes directory no longer has cost-aware-profile-selection
- [x] Verbatim diff-r readback output is empty (no differences) — mechanical copy verified
- [x] Sibling dependency (agnostic-role-profiles) handled: base specs merged, NOT archived
- [x] All Engram observation IDs recorded for traceability

## SDD Cycle Complete

The cost-aware profile selection change has been fully planned (proposal), specified (delta specs with 4 domains), designed (6 load-bearing decisions, PR boundary rationale), implemented (3 chained PRs with 50 complete tasks, 622+ authored lines), verified (PASS, 138/138 tests, golden parity), and archived. The feature ships in v2.4.0 with:

- Three ranking objectives (value/quality/budget) selectable per-profile or per-role
- Two currencies (usd price-based, quota request-budget-based) with frequency-aware affordability weighting
- Tier capture in the setup wizard (go/zen → quota vs usd default)
- Embedded quota table (OpenCode models → requests/5h) with staleness surfacing
- Full backwards compatibility: default unmodified profiles rank identically to pre-change behavior

Ready for the next change.

---

**Archive Report Generated**: 2026-08-21  
**Skill**: sdd-archive/2.0  
**Mode**: hybrid (openspec + engram)  
**Mechanical Operations**: cp -R snapshot, git mv tracked folder, diff -r verification (empty output)
