# Proposal: Derive Plan Quota at Request Time from Live Pricing

## Intent

Most somm users are on OpenCode **Go** ($10/mo). `currency: quota` exists so recommendations respect what a Go subscriber can actually afford to call. The data behind it — `opencode-zen-go.yaml` — covers 4 of ~20 Go models and is already stale (`deepseek-v4-pro` embedded at 200 req/5h vs 1,050 today; `gpt-5.5-pro` renamed to `gpt-5.6-luna` and no longer resolves). Every price change silently degrades quota ranking with no signal.

The fix is to stop storing the volatile number. Store only the low-churn inputs — a per-request token **shape**, a per-model **multiplier**, and the per-model **included-usage tier** — and compute requests-per-window at request time from prices somm already fetches. No extra network call, no per-session doc re-reading, no token cost.

## Scope

### In Scope
- Replace `requests_per_5h` in `opencode-zen-go.yaml` with shape (input/cached/output tokens) + `multiplier` + included-usage tier, covering all ~20 Go models.
- Rework `Table.Requests()` into a price-aware resolver; rework `resolveQuotaDenominators` to compute from (shape, multiplier, tier, live price).
- Wire the already-parsed-but-discarded `InputCacheRead` into `Money` / `enrichWithOpenRouter`.
- Restate `buildReason` staleness text: curation date of shape/tier, not "measured" quota.
- Table-driven tests for the resolver, denominator, fallback, and reason (none exist today).

### Out of Scope
- Changing the ranking key `Qraw / denominator` or `H_sat`/`frequency_weight` semantics.
- Image-token billing for DeepSeek Vision; account-wide $12/$30/$60 ceiling enforcement.
- A new network source for prices. Adding a `PROVIDER_ALIASES` entry for Muse Spark.

## Capabilities

### New Capabilities
- None

### Modified Capabilities
- `plan-quota-currency`: "Embedded Quota Table" becomes an embedded **shape/tier** table; "Staleness Surfacing" claims curation date of assumptions, not measurement of a quota; "Fallback for Untabulated Models" re-verified against the derived denominator.

## Approach

Validated month-window formula: `requests_per_month ≈ included_tier_usd / cost_per_request(shape, price)` (off-peak prices where variants exist). Verified near-exact for Grok (600 vs 600.7), Hy3 base (21,500 vs 21,507), DeepSeek V4 Pro (5,200 vs 5,203). Residual 10–36% drift on some models traces to OpenCode's own rounded "observed pattern" shapes — acceptable for a *relative ranking* denominator, which is the only consumer.

## Open Decisions for Design

| # | Decision | Why unresolved |
|---|---|---|
| D1 | 5h/week window derivation | Month/week is a stable ~2.0x; month/5h ranges 4.45–5.06x. No exact universal divisor exists in public data. Options: (a) documented approximate divisors, (b) derive month live + keep 5h/week curated, (c) design finds better. |
| D2 | Price source trust | OC Go endpoint carries no price fields; `currency: usd` uses OpenRouter's price for the *base* model. Spike required: log one raw OC Go JSON body to check for undecoded pricing before committing to the OpenRouter proxy. |
| D3 | Muse Spark 1.2 Contributor | Has shape+tier but no OpenRouter alias, so no live price. Confirm (do not assume) that falling through to the existing `price / P_min` bridge is acceptable. |
| D4 | Tiered/peak prices | GPT-5.6 Luna (≤272K), Qwen3.7/3.6 Plus (≤256K), DeepSeek peak/off-peak — OpenRouter reports one number; which variant is unverified. |

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/profile/plans/opencode-zen-go.yaml` | Modified | Schema swap: shape + multiplier + tier; ~20 models |
| `internal/profile/plans/plans.go` | Modified | `Table` struct + `Requests()` API becomes price-aware |
| `internal/api/models.go` | Modified | `Money` gains cache-read field |
| `internal/api/match.go` | Modified | Copy `InputCacheRead` in `enrichWithOpenRouter` |
| `internal/api/recommend.go` | Modified | `resolveQuotaDenominators`, `buildReason` |
| `openspec/specs/plan-quota-currency/spec.md` | Modified | Two requirements reworded |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| OpenRouter price ≠ OpenCode Go price → silent ranking drift | Med | D2 spike before design locks the formula; document the proxy caveat in spec |
| Approximate 5h/week divisor diverges from OpenCode's displayed numbers | High | Denominator feeds relative ranking, not a displayed SLA; D1 decides explicitly |
| Rewrite regresses quota ranking with no test net | Med | Tests land before behavior change (repo TDD is on) |
| Runtime cost/complexity added to hot path | Low | Pure arithmetic over already-fetched prices; no I/O |

## Rollback Plan

Single-commit revert restores the static `requests_per_5h` table and the old `Requests()` signature; no persisted state, migrations, or on-disk user config are touched. If only the derivation misbehaves, the table can be pinned by hardcoding computed values back into the YAML without reverting the plumbing.

## Dependencies

- Reference snapshot of OpenCode Go docs data: `openspec/changes/live-quota-derivation/reference-data.md` (not re-fetchable; authoritative curation input).
- D2 spike result before `sdd-design` finalizes the price input.

## Success Criteria

- [ ] All ~20 Go models resolve a quota denominator; none silently fall back for a missing table entry.
- [ ] Derived month-window values reproduce the docs' published estimates within the documented tolerance for the validated models (Grok, Hy3, DeepSeek V4 Pro, MiniMax M3).
- [ ] A price change alone updates quota ranking with no YAML edit.
- [ ] `Reason` text no longer claims the quota number was "measured" on a date.
- [ ] `go test -cover ./...` passes with new table-driven tests on the resolver, denominator, fallback, and reason.
