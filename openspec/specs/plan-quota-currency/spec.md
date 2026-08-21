# Plan Quota Currency Specification

## Purpose

Define the embedded OpenCode plan quota table, its staleness surfacing, the bounded
per-role frequency weighting, and the `quota` currency denominator consumed by
`weighted-scoring`. The denominator is a saturating **affordability factor**, not a raw
quota-derived value: an unbounded formula is provably order-preserving for `frequency` and
never changes a role's winner (see Requirement: Frequency Weighting), so the factor MUST be
capped at a fixed saturation constant to actually influence ranking.

## Requirements

### Requirement: Embedded Quota Table

The system MUST embed a curated quota table (mirroring the `presets/` embed pattern),
mapping OpenCode model id (`ocId`) to `requests_per_5h` for at least one plan tier, MUST
carry a single `measured_at` date for the whole table, and MUST be loadable without any
network or filesystem access outside the binary.

#### Scenario: Table resolves a known model id

- GIVEN the embedded table contains an entry for a given `ocId`
- WHEN the table is queried for that `ocId`
- THEN the resolver MUST return its `requests_per_5h` value

#### Scenario: Table omits an unknown model id

- GIVEN a candidate `ocId` absent from the table
- WHEN the table is queried
- THEN the resolver MUST report "not found" rather than a zero or default value

### Requirement: Frequency Weighting

The system MUST compute each candidate's quota denominator as
`denominator = H_sat / min(requests_per_5h / frequency_weight(role), H_sat)`, where
`H_sat = 200` is a fixed saturation constant and `frequency_weight` maps the role's
effective `frequency` to a fixed multiplier: `high -> 4`, `medium -> 1`, `low -> 0.25`. The
ranking key remains `Qraw / denominator` (weighted-scoring, Requirement:
Currency-Selected Denominator). The saturation cap is load-bearing: without it,
`frequency_weight` is a per-role constant that cancels out of every `Qraw / denominator`
comparison and never changes the winner within that role — `H_sat` is what makes
`frequency` structurally able to invert a ranking.

#### Scenario: Higher frequency weight favors quota-abundant models

- GIVEN premium candidate P (`Qraw=80, requests_per_5h=110`) and cheap-abundant candidate C
  (`Qraw=50, requests_per_5h=45300`), both under `currency: quota`
- WHEN ranked for a `frequency: high` role (`frequency_weight=4`)
- THEN `denominator_P = 200 / min(110/4, 200) = 200/27.5 = 7.27`, ranking key
  `80/7.27 = 11.00`; `denominator_C = 200 / min(45300/4, 200) = 200/200 = 1.0`, ranking key
  `50/1.0 = 50.00`; C MUST win
- WHEN the same two candidates are ranked for a `frequency: low` role
  (`frequency_weight=0.25`)
- THEN `denominator_P = 200 / min(110/0.25, 200) = 200/200 = 1.0`, ranking key
  `80/1.0 = 80.00`; `denominator_C = 200 / min(45300/0.25, 200) = 200/200 = 1.0`, ranking key
  `50/1.0 = 50.00`; P MUST win, inverting the `high`-frequency winner

### Requirement: Fallback for Untabulated Models

When a candidate's `ocId` is absent from the embedded quota table, the system MUST
substitute an affordability-equivalent denominator `denominator = price / P_min`, where
`P_min` is the minimum price over the role's constraint-filtered candidate set — the same
min-max normalization precedent used by `computeNormalized` (weighted-scoring). This keeps
untabulated candidates on the same bounded scale as quota-ranked candidates (both denominators
saturate near `1.0` for the most favorable candidate); a raw `usd`-price sort MUST NOT be
mixed directly into a quota-scaled ranking, since the two scales differ by orders of
magnitude and would make the fallback candidate always win or always lose regardless of
merit. The system MUST NOT raise an error or exclude the candidate solely for the missing
table entry.

#### Scenario: Untabulated model still receives a comparable rank

- GIVEN `currency: quota`, a role's constraint-filtered candidate set with cheapest price
  `P_min = 2.0`, and a candidate absent from the quota table priced at `4.0`
- WHEN that candidate's denominator is resolved
- THEN `denominator = 4.0 / 2.0 = 2.0`, used in the same `Qraw / denominator` ranking key as
  quota-tabulated candidates — not a raw, unscaled `usd` price

### Requirement: Staleness Surfacing

Every recommendation whose denominator was resolved via the quota table (not the
untabulated fallback) MUST include the table's `measured_at` date in its `Reason` text.

#### Scenario: Quota-ranked reason carries measured_at

- GIVEN a role under `currency: quota` whose winning candidate came from the quota table
- WHEN the recommendation `Reason` is built
- THEN the `Reason` text MUST contain the table's `measured_at` date

#### Scenario: Price-fallback reason omits quota staleness claim

- GIVEN a role under `currency: quota` whose winning candidate fell back to the
  `price / P_min` denominator (Requirement: Fallback for Untabulated Models)
- WHEN the recommendation `Reason` is built
- THEN the `Reason` text MUST NOT claim a `measured_at` date for that candidate's denominator
