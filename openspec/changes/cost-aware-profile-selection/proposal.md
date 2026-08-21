# Proposal: Cost-Aware Profile Selection

## Intent

`somm` can only express one ranking objective. In `findBestModel`
(`internal/api/recommend.go:161`) a role with `weights` is *always* ranked by
`Qraw / price` descending; only weightless roles get the price-ascending branch.
A user cannot ask for "best available, price as tiebreak" for a critical role —
"premium" is unsayable in the schema.

Worse, `price` is the wrong denominator for the users this tool targets. On a
flat-fee tier (OpenCode Go, $10/mo) the binding constraint is a per-model
request bucket ("requests per 5 hours"), which runs roughly *inverse* to sticker
price (Kimi K3: 110 req/5h; Muse Spark 1.2: 45,300 req/5h). Optimizing $/token
for a subscriber optimizes a cost they already paid.

## Scope

### In Scope
- **Slice 1 — `selection.objective`**: `value` (default, today's ratio) |
  `quality` (Qraw desc, price as tiebreak) | `budget` (`value` +
  `defaults.max_input_price` as a hard floor). Profile-level, role-overridable.
- **Slice 2 — `selection.currency: quota`**: denominator becomes
  `1 / (requests_per_5h) * frequency_weight(role)`; new per-role
  `frequency: high|medium|low`, read only under `quota` and orthogonal to
  `criticidad`.
- Curated embedded quota table (`internal/profile/plans/opencode-zen-go.yaml`,
  mirroring the `presets/` embed pattern) with a `measured_at:` date surfaced in
  the recommendation `Reason` for staleness visibility.
- **Tier capture in the setup wizard** (decision D1): `somm setup` asks the
  OpenCode tier once and persists it, so `currency` inference is never a guess
  and never re-prompts.
- Defaults stay `objective: value` / `currency: usd` — byte-for-byte parity with
  today's `gentle-ai` output.

### Out of Scope
- Role-taxonomy auto-detection (`somm profile init --detect` scanning
  `.claude/agents/*`) — separate concern, later change.
- Vendoring third-party role taxonomies as presets — permanent maintenance cost.
- Scraping OpenCode's authenticated usage dashboard. Confirmed this session:
  the quota table is published only as a webpage, on no documented API endpoint.
  The table is manual by necessity.

## Capabilities

### New Capabilities
- `plan-quota-currency`: embedded plan quota table, `measured_at` staleness,
  frequency weighting, and the quota denominator.

### Modified Capabilities
- `role-profiles`: adds the `selection` block and per-role `frequency`.
- `weighted-scoring`: ranking objective becomes selectable; `Qraw/price` becomes
  one of three objectives rather than the only one.

## Decisions

| ID | Decision | Rationale |
|----|----------|-----------|
| D1 | Tier is captured by an explicit question in `somm setup`, persisted once, never re-asked | Inferring from the `Subscription: go\|zen\|both` stamp is automatic but wrong: it reports which *catalog* a model appears in, not which tier the user *pays for*. A bad guess silently selects the wrong `currency` and misranks every recommendation — a real cost borne by the user. An asked-once, cached answer is both automatic and correct. |
| D2 | Both slices ship in this change as 2 chained PRs, stacked-to-main | Matches the `agnostic-role-profiles` precedent; slice 1 is small and self-contained, slice 2 carries the line-count risk against the 400-line review budget. |
| D3 | `selection` is a profile-level block with per-role override | Lets one profile mix a `quality`-objective critical judge with `value`-objective workhorse roles — the common real configuration. |

## Approach

Add `Selection{Objective, Currency}` to `profile.Profile` and `Frequency` to
`profile.Role`, both defaulted and validated in `Load`. Per D3, a role-level
`selection` overrides the profile-level block field-by-field. In
`findBestModel`, replace the single hard-coded comparator with an
objective-selected one; the hard-constraint pre-filter, max-2/relax-3
spreading, and `Qraw` (never `Qnorm`) invariants are untouched. A `plans`
package resolves `ocId -> requests_per_5h`; any model absent from the table
falls back to price-based ranking and never errors. `buildReason` reports the
active objective, currency, and `measured_at`.

Per D1, the tier is collected as a new wizard step and persisted to `.env`
through the existing `saveEnvFile(envPath, keys)` (e.g. `SOMM_OC_TIER=go|zen`),
alongside the two API keys. It is deliberately **not** written into
`openCodeConfig`: that type holds `opencode.json` raw and only ever touches the
`mcp.somm` key by design (`cmd/somm/setup.go:20-29`). An explicit
`selection.currency` in a profile always wins over the persisted tier.

## Delivery

Two chained PRs, stacked-to-main (D2), each under the 400-line review budget:

1. **PR1 — objective**: `Selection` schema + validation + override resolution,
   objective comparator in `findBestModel`, `buildReason` objective text, tests.
2. **PR2 — quota currency**: `plans` package + embedded table, `Role.Frequency`,
   quota denominator, `measured_at` in `Reason`, setup-wizard tier step, tests.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/profile/profile.go` | Modified | `Selection` type, role override, `Role.Frequency`, validation |
| `internal/profile/plans/` | New | Embedded quota table + loader (PR2) |
| `internal/api/recommend.go` | Modified | Objective comparator, quota denominator, `buildReason` |
| `cmd/somm/tui.go`, `cmd/somm/setup.go` | Modified | Tier wizard step; persist via `saveEnvFile` (PR2, D1) |
| `cmd/somm/main.go` | Modified | Read persisted tier, derive default `currency` |
| `internal/profile/presets/gentle-ai.yaml` | Unchanged | Parity preserved via defaults |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Default output silently changes for existing users | Med | Golden test locks `gentle-ai` default; `value`/`usd` is a no-op path |
| Quota table goes stale and misleads | High | `measured_at` in every quota-ranked `Reason`; missing entry falls back to price |
| Wrong tier → wrong denominator | Low | D1 asks instead of guessing; explicit `selection.currency` always overrides |
| Wizard gains a step users must answer | Low | Asked once, persisted to `.env`, skipped on every later run |
| Two axes × three objectives inflates the review diff | Med | Two chained PRs, stacked-to-main (D2) |
| **Sibling change unarchived; deltas stack on the same capabilities** | **Med** | See Dependencies — base this change's deltas on the sibling's delta files, and run a real `/sdd-verify` + `/sdd-archive` on it before or shortly after this lands |

## Rollback Plan

Both PRs are additive behind defaults. Revert the `cost-aware-profile-selection`
commits; profiles carrying a `selection:` block then fail validation as unknown
keys, so a revert should pair with the documented note that users drop the block.
The persisted `SOMM_OC_TIER` in `.env` becomes an inert unread key — harmless,
no migration needed. Reverting PR2 alone leaves PR1's objective axis working.

## Dependencies

- `agnostic-role-profiles` (`internal/profile`, `Profile`/`Role`/`Resolve`).
  **Code-complete and healthy** — `go build ./... && go test ./...` was
  independently verified green across all packages. But the change is **not
  archived**: its `verify-report.md` is a 10-line stub (literally "test line
  one" / "test line two with apostrophe it's fine") despite every task being
  checked off in `tasks.md`, so its spec deltas still live under
  `openspec/changes/agnostic-role-profiles/specs/` and were never merged into
  `openspec/specs/`.
  - **Consequence for this change**: our deltas for `role-profiles` and
    `weighted-scoring` MUST base themselves on the sibling's delta files as the
    current source of truth, not on `openspec/specs/`.
  - **Recommended (not owned by this change)**: run a genuine `/sdd-verify` on
    `agnostic-role-profiles`, replacing the bogus report, then `/sdd-archive`
    it — before this change lands, or shortly after. Otherwise two unarchived
    delta sets stack on the same two capabilities and archive-time merge order
    becomes ambiguous. This proposal deliberately does not fix or archive the
    sibling change itself.

## Success Criteria

- [ ] Default run (no `selection` block) reproduces today's output byte-for-byte.
- [ ] `objective: quality` on a critical role picks the top-`Qraw` candidate, not
      the best ratio, with price used only to break ties.
- [ ] `objective: budget` never returns a candidate above `max_input_price`.
- [ ] `currency: quota` ranks a high-quota cheap model above a low-quota premium
      one for a `frequency: high` role, and inverts for `frequency: low`.
- [ ] A model missing from the quota table falls back to price ranking, no error.
- [ ] Every quota-ranked recommendation `Reason` carries `measured_at`.
- [ ] A role-level `selection` overrides the profile-level block field-by-field.
- [ ] `somm setup` asks the tier once, persists it, and does not re-ask on a
      later run; an explicit `selection.currency` still overrides it.

## Proposal question round — RESOLVED

All three blocking questions were answered by the user. Outcomes are recorded as
D1–D3 in **Decisions** above and folded into Scope, Approach, Delivery, Affected
Areas, Risks, and Success Criteria. Nothing in this proposal is pending.

| Q | Question | Answer |
|---|----------|--------|
| Q1 | How does somm learn the user's tier? | Ask once in `somm setup`, persist, never re-ask (D1) — rejected inference from the `Subscription` stamp as unreliable |
| Q2 | Slice 2 here or fast-follow? | Here: 2 chained PRs, stacked-to-main (D2) |
| Q3 | Is `selection` role-overridable? | Yes: profile-level block with per-role override (D3) |

### Confirmed assumptions
1. Defaults never change for existing users; both axes are opt-in.
2. `objective` and `currency` are independent axes, not a single enum.
3. `frequency` is ignored entirely under `currency: usd` (not an error).
4. The quota table is hand-maintained and intentionally incomplete.
