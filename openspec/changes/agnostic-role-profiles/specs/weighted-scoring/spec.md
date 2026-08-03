# Weighted Scoring Specification

## Purpose

Define how candidate models are filtered by hard constraints and scored per role using a
weighted sum, including the parity-critical rule that the quality/price ratio always uses
the raw weighted sum (never a normalized value), and the preserved assignment-spreading and
relaxation behavior.

## Requirements

### Requirement: Hard Constraint Pre-Filter

For each role, the system MUST filter candidates, per assignment attempt, in this order:
(1) skip models with no usable pricing, (2) skip models already at the per-model assignment
cap, (3) apply hard constraints `min_context`, `max_input_price`, `requires`, and
`exclude_family_of`, (4) for weighted roles, skip candidates with a `nil` value on any
positively-weighted metric. Hard constraints MUST NOT be relaxed at any point.

#### Scenario: `min_context` and `max_input_price` filter candidates

- GIVEN a role with `min_context: 100000` and `max_input_price: 5.0`, and candidates with
  context lengths and prices both above and below those bounds
- WHEN candidates are filtered for the role
- THEN only candidates meeting both constraints MUST remain in the candidate set

#### Scenario: `exclude_family_of` excludes a family already assigned

- GIVEN role `B` has `exclude_family_of: A`, and role `A` was already assigned a model whose
  family is `openai`
- WHEN candidates are filtered for role `B`
- THEN candidates whose family is `openai` MUST be excluded from role `B`'s candidate set

### Requirement: Raw Weighted Sum Drives the Quality/Price Ratio

The system MUST compute `Qraw = Σ (weight_m * raw_m)` per candidate per role. The
quality/price ratio used for ranking and the winner tiebreak MUST be `Qraw / price` and
`Qraw` descending, respectively — never a normalized quantity. This preserves today's
raw-ratio ordering; normalization MUST NOT be applied before the ratio is computed.

#### Scenario: Raw ratio ordering matches pre-refactor behavior

- GIVEN candidate A with `intelligence=72.5, price=30` and candidate B with
  `intelligence=52.1, price=0.95`, both scored by a single-weight `intelligence` role
- WHEN the role's ranking is computed
- THEN candidate B MUST win (`Qraw/price`: A=2.417, B=54.8), matching the raw-ratio result,
  not the normalize-then-ratio result

### Requirement: Multi-Metric Normalization

For roles weighting two or more metrics, the system MUST also compute a normalized value
`norm_m = (raw_m - min_m) / (max_m - min_m)` per metric, per role, over that role's
constraint-filtered candidate set, and `Qnorm = Σ (weight_m * norm_m)`. `Qnorm` MUST be used
only as an internal quality comparison for multi-metric roles and MUST NOT replace `Qraw` in
the price ratio. When `max_m == min_m` for a metric (including a single-candidate set),
`norm_m` MUST be `1.0` for all candidates on that metric (no division by zero).

#### Scenario: Single candidate normalizes to neutral value

- GIVEN a role with two weighted metrics and exactly one candidate remaining after the
  constraint pre-filter
- WHEN normalization is computed
- THEN both `norm_m` values for that candidate MUST be `1.0`

#### Scenario: All-equal candidates normalize to neutral value

- GIVEN a role with two weighted metrics and three candidates that all share the same raw
  value on one metric
- WHEN normalization is computed for that metric
- THEN `norm_m` MUST be `1.0` for all three candidates on that metric

### Requirement: Empty Candidate Set Fallback

When a role's constraint-filtered candidate set is empty at the current assignment cap, the
system MUST attempt the existing 2-to-3 assignment-cap relaxation before treating the role as
unassignable. Hard constraints MUST remain applied during relaxation. If the candidate set is
still empty after relaxation, the system MUST report the role using today's per-role
"no model available" reason path rather than raising a hard error.

#### Scenario: Cap relaxation recovers a candidate

- GIVEN a role whose candidate set is empty at assignment cap 2 solely because all
  unconstrained candidates are already assigned twice
- WHEN relaxation to cap 3 is applied
- THEN a previously-excluded candidate at cap 2 but available at cap 3 MUST be considered

#### Scenario: Constraints alone empty the set, no relaxation recovers it

- GIVEN a role with `min_context: 1000000` that no candidate satisfies, even after cap
  relaxation
- WHEN scoring completes for that role
- THEN the role MUST surface the existing "no model available" reason, not a fatal error

### Requirement: Assignment Spreading Preserved

The system MUST preserve the existing max-2-per-model assignment rule with relax-to-3
fallback across all roles in a single recommendation run, independent of profile source.

#### Scenario: A model is not assigned to more than 3 roles

- GIVEN a profile where multiple roles independently prefer the same top-ranked model
- WHEN recommendations are computed for all roles in the run
- THEN no single model MUST be assigned to more than 3 roles, and no model reaches a 3rd
  assignment unless cap-2 relaxation was necessary for that role
