# Delta for Plan Quota Currency

## MODIFIED Requirements

### Requirement: Embedded Quota Table

The system MUST embed a curated table (mirroring the `presets/` embed pattern) mapping
each OpenCode Go `ocId` to a per-request token **shape** (input/cached/output counts), an
optional **multiplier** (default `1`), and an **included-usage tier** in USD, plus a single
curation date for the table. It MUST be loadable with no network/filesystem access outside
the binary. The table MUST NOT store a final requests-per-window number; that number MUST be
derived at request time from these inputs plus the model's live price, as
`effective_budget_usd = tier_usd * multiplier`, then divided by cost-per-request.
(Previously: stored a final `requests_per_5h` integer per model.)

#### Scenario: Table resolves a known model's shape and tier

- GIVEN the table has `grok-4.5` (shape `1100/71500/220`, tier `$15`, no multiplier)
- WHEN queried for `grok-4.5`
- THEN it MUST return the shape and tier, with multiplier defaulting to `1`

#### Scenario: Table omits an unknown model id

- GIVEN an `ocId` absent from the table
- WHEN queried
- THEN it MUST report "not found", never a zero/default value

#### Scenario: Multiplier scales window budget — Hy3, 8x

- GIVEN `hy3` (shape `830/71500/295`, tier `$60`, multiplier `8`), price `0.14/0.58/0.035`,
  `cost_per_request ≈ $0.0027898`
- WHEN the window quota is resolved
- THEN `effective_budget = $480`, `quota ≈ 172,058` — 8x the multiplier-1 baseline
  (`~21,507`), matching OpenCode's "8x usage" note

### Requirement: Fallback for Untabulated Models

When a candidate's `ocId` is absent from the table, the system MUST substitute
`denominator = price / P_min` (`P_min` = cheapest price in the role's constraint-filtered
set — the `computeNormalized` precedent), keeping it on the same bounded scale as
quota-ranked candidates; a raw `usd` price MUST NOT be mixed in directly. The system MUST
NOT error or panic solely for a missing table entry.

A candidate with no resolvable live price — whether its `ocId` is table-present or
table-absent — MUST be excluded from candidacy before quota resolution runs, the same as
under any other currency (a price is required to rank at all). It therefore never reaches
this fallback and never triggers a panic on that account.
(Previously: triggered only on a missing `ocId`; a table-present candidate with no live
price was undefined behavior, not yet distinguished from the table-absent case.)

#### Scenario: Untabulated model still receives a comparable rank

- GIVEN `currency: quota`, role `P_min = 2.0`, candidate absent from the table priced `4.0`
- WHEN its denominator is resolved
- THEN `denominator = 4.0/2.0 = 2.0`, used in the same `Qraw/denominator` key — not a raw
  `usd` price

#### Scenario: Curated model with no resolvable live price is excluded from candidacy

- GIVEN `currency: quota` and a table model with resolvable shape/tier but no live price —
  either `muse-spark-1.2-contributor` (shape `620/71400/300`, tier `$60`, no price-source
  match) or any table model whose price fetch failed/returned nil this request
- WHEN candidates are collected
- THEN the system MUST exclude it from the candidate set before quota resolution runs,
  MUST NOT crash or panic, and MUST NOT reach the `price/P_min` fallback — that fallback
  itself requires a price the model does not have

### Requirement: Staleness Surfacing

Every recommendation whose denominator was resolved via the table and a live price (not the
fallback) MUST include the table's curation date in its `Reason` text, described as when the
shape/multiplier/tier data was curated — never as when the shown request count was measured,
since that count is now computed at request time.
(Previously: claimed the shown request count itself was "measured" on the table's date.)

#### Scenario: Quota-ranked reason carries curation date, not a measurement claim

- GIVEN a `currency: quota` role whose winner resolved via the table and a live price
  (e.g. `grok-4.5`)
- WHEN `Reason` is built
- THEN it MUST contain the curation date, phrased as when the inputs were curated, and MUST
  NOT assert the request count itself was measured

#### Scenario: Price-fallback reason omits quota staleness claim

- GIVEN a `currency: quota` role whose winner fell back to `price/P_min`
- WHEN `Reason` is built
- THEN it MUST NOT reference the curation date or claim any curated/measured quota basis

## ADDED Requirements

### Requirement: Cache-Read Price Included in Cost Calculation

The system MUST include the model's live cache-read price (`input_cache_read`) as a term in
cost-per-request, weighted by the shape's cached-token count like input/output prices. The
table MUST also curate a per-model `cache_read_ratio` (cache-read price ÷ input price). When
the live cache-read price is nil but input/output resolve, the system MUST substitute
`cache_read_ratio * live_input_price` as the cache-read term instead of failing the
calculation or triggering the untabulated-model fallback — never a hard `0` unless the
curated ratio itself is unset.

#### Scenario: Cache-read price contributes to cost — Grok 4.5

- GIVEN `grok-4.5` (shape `1100/71500/220`, tier `$15`), price input `$2.00`, output `$6.00`,
  cache-read `$0.30` (per 1M)
- WHEN cost-per-request is computed
- THEN `cost = (1100*2.00 + 71500*0.30 + 220*6.00)/1e6 = $0.02497`
- AND window quota `≈ 600.7`, matching OpenCode's published month estimate `600` within
  validated tolerance

#### Scenario: Missing live cache-read price falls back to curated ratio

- GIVEN a table model with a curated `cache_read_ratio`, resolvable input/output prices, but
  no live cache-read price from the price source this request
- WHEN cost-per-request is computed
- THEN the cache-read term MUST be `cache_read_ratio * live_input_price`, not a literal
  zero, and the quota MUST resolve without error or fallback trigger

## Notes for Design (resolved)

D1-D5 were resolved during `sdd-design` (see `design.md`) and this delta reflects those
resolutions: D1 — window divisors are exactly `tier/5` (5h), `tier/2` (week), `tier/1`
(month). D2 — OpenRouter accepted as a documented, mitigated price-proxy risk. D3 — a
curated-but-unpriced model is excluded from candidacy, not fallback-ranked (see Fallback
scenario above). D4 — off-peak/lower-tier pricing assumed, no variant detection. D5 —
curated `cache_read_ratio` fallback when live cache-read price is nil (see Cache-Read
Requirement above).
