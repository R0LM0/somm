# Delta for Weighted Scoring

## MODIFIED Requirements

### Requirement: Raw Weighted Sum Drives the Quality/Price Ratio

The system MUST compute `Qraw = Σ (weight_m * raw_m)` per candidate per role. The ranking
denominator MUST be selected per the role's effective `currency` (see Requirement:
Currency-Selected Denominator). Under the `value` objective (the default), the ranking key
and winner tiebreak MUST be `Qraw / denominator` descending and `Qraw` descending
respectively — never a normalized quantity; normalization MUST NOT be applied before the
ratio is computed. Under `quality` or `budget` objectives, ranking uses the comparator
defined in Requirement: Objective-Selected Comparator instead.

(Previously: `Qraw / price` was the only ranking key for every weighted role — there was no
`quality`/`budget` objective and no alternate currency denominator.)

#### Scenario: Raw ratio ordering matches pre-refactor behavior under the default objective

- GIVEN candidate A with `intelligence=72.5, price=30` and candidate B with
  `intelligence=52.1, price=0.95`, both scored by a single-weight `intelligence` role with no
  `selection` block (defaults: `objective: value`, `currency: usd`)
- WHEN the role's ranking is computed
- THEN candidate B MUST win (`Qraw/price`: A=2.417, B=54.8), identical to pre-change behavior

## ADDED Requirements

### Requirement: Objective-Selected Comparator

The system MUST select the ranking comparator per role from its effective `objective`
(Requirement: Selection Block Schema, `role-profiles`): `value` sorts by `Qraw / denominator`
descending, tiebreak `Qraw` descending (existing behavior, unchanged); `quality` sorts by
`Qraw` descending, tiebreak `denominator` ascending; `budget` uses the `value` comparator
restricted to candidates whose price does not exceed the role's effective `max_input_price`,
applied as an additional hard pre-filter rather than a soft preference. `role-profiles`
(Requirement: Budget Objective Requires an Effective Ceiling) guarantees `max_input_price` is
always present whenever the effective objective is `budget`, so this comparator MUST NOT
silently degrade to the `value` comparator for lack of a ceiling. The hard-constraint
pre-filter, max-2/relax-3 assignment spreading, and the invariant that ranking is always
driven by `Qraw` (never `Qnorm`) remain unchanged for all three objectives.

#### Scenario: quality objective picks top-Qraw candidate over better ratio

- GIVEN a `quality`-objective role with candidate A (`Qraw=90, price=50`) and candidate B
  (`Qraw=80, price=5`)
- WHEN the role's ranking is computed
- THEN candidate A MUST win despite B having the better `Qraw/price` ratio

#### Scenario: budget objective never exceeds max_input_price

- GIVEN a `budget`-objective role with `max_input_price: 5.0` and a candidate priced above
  that ceiling holding the best `Qraw/price` ratio
- WHEN candidates are filtered and ranked for the role
- THEN that candidate MUST be excluded from the result, and the winner MUST be the best
  `value`-ranked candidate at or below the ceiling

#### Scenario: quality objective ties broken by lower denominator

- GIVEN a `quality`-objective role with two candidates sharing the same `Qraw` and different
  prices
- WHEN the role's ranking is computed
- THEN the candidate with the lower price MUST win the tie

### Requirement: Currency-Selected Denominator

The system MUST select the ranking denominator from the role's effective `currency`
(Requirement: Selection Block Schema, `role-profiles`): `usd` MUST use `price` (existing
behavior, unchanged); `quota` MUST use the saturating affordability denominator
`H_sat / min(requests_per_5h / frequency_weight(role), H_sat)` with fixed constant
`H_sat = 200` (`plan-quota-currency`, Requirement: Frequency Weighting) — an unsaturated
`1 / requests_per_5h * frequency_weight(role)` formula is provably order-preserving for
`frequency_weight` within a role and never changes the winner, so the cap is load-bearing,
not cosmetic. A candidate absent from the quota table MUST use the bridged denominator
`price / P_min` (`plan-quota-currency`, Requirement: Fallback for Untabulated Models) instead
of a raw, unscaled `usd` price, so it stays on the same bounded scale as quota-ranked
candidates rather than winning or losing purely from unit mismatch.

#### Scenario: quota currency ranks high-quota cheap model above low-quota premium model

- GIVEN premium candidate P (`Qraw=80, requests_per_5h=110`) and cheap-abundant candidate C
  (`Qraw=50, requests_per_5h=45300`) under a `frequency: high` role (`frequency_weight=4`)
- WHEN the role's ranking is computed
- THEN `denominator_P = 200/min(110/4,200) = 7.27` (key `11.00`) and
  `denominator_C = 200/min(45300/4,200) = 1.0` (key `50.00`); C MUST win

#### Scenario: model missing from quota table uses the bridged denominator, not raw price

- GIVEN `currency: quota`, a role's cheapest constraint-filtered candidate priced at
  `P_min = 2.0`, and a candidate absent from the embedded quota table priced at `4.0`
- WHEN that candidate's denominator is resolved
- THEN it MUST be `4.0 / 2.0 = 2.0`, not the raw `usd` price, and it MUST NOT error or be
  excluded solely for the missing table entry
