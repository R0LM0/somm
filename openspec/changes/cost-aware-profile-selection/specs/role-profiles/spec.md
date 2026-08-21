# Delta for Role Profiles

## MODIFIED Requirements

### Requirement: Profile Schema

The system MUST accept a YAML `Profile` document containing a `version`, an optional
`defaults` block, an optional top-level `selection` block (see Requirement: Selection
Block Schema), and a list of `roles`. Each role MUST declare an `id` and MAY declare
`description`, `criticidad`, `weights` (a map of metric key to float64, keys restricted to
`intelligence`, `coding`, `agentic`), `min_context` (int64), `max_input_price` (float64),
`requires` (a list of capability tokens), `exclude_family_of` (a role id string), `selection`
(a role-level override of the profile-level block, see Requirement: Per-Role Selection
Override), and `frequency` (see Requirement: Frequency Field).

(Previously: profile schema had no `selection` block anywhere and roles had no `frequency`
field.)

#### Scenario: Valid multi-metric profile loads

- GIVEN a YAML file with one role weighting `intelligence: 0.6` and `coding: 0.4`, plus
  `min_context: 100000` and `requires: ["reasoning"]`
- WHEN the profile is loaded
- THEN the resulting `Role` has both weights set and both hard constraints populated

#### Scenario: Unknown metric key is rejected

- GIVEN a YAML file with `weights: { creativity: 1.0 }`
- WHEN the profile is loaded
- THEN loading MUST fail with a validation error naming the unknown metric key

## ADDED Requirements

### Requirement: Selection Block Schema

The system MUST accept an optional `selection` block, at the profile level and/or per-role,
with two independent fields: `objective` (`value` | `quality` | `budget`, default `value`)
and `currency` (`usd` | `quota`, default `usd`). An unrecognized `objective` or `currency`
value MUST fail validation. When `selection` is absent entirely, resolved behavior MUST be
byte-for-byte identical to a profile with no `selection` block.

#### Scenario: Default profile has no selection block

- GIVEN a profile with no `selection` key anywhere
- WHEN the profile loads
- THEN the effective objective MUST be `value` and the effective currency MUST be `usd` for
  every role

#### Scenario: Unknown objective value rejected

- GIVEN `selection: { objective: cheapest }`
- WHEN the profile loads
- THEN loading MUST fail with a validation error naming the unknown objective value

### Requirement: Per-Role Selection Override

Per decision D3, a role-level `selection` block MUST override the profile-level `selection`
block field-by-field: a role that sets only `objective` MUST inherit the profile-level
`currency`, and vice versa.

#### Scenario: Role overrides only objective, inherits currency

- GIVEN profile-level `selection: { objective: value, currency: quota }` and a role with
  `selection: { objective: quality }`
- WHEN the role resolves its effective selection
- THEN the role's effective objective MUST be `quality` and its effective currency MUST be
  `quota` (inherited)

#### Scenario: Role selection absent, profile-level selection applies fully

- GIVEN profile-level `selection: { objective: budget, currency: usd }` and a role with no
  `selection` block
- WHEN the role resolves its effective selection
- THEN the role's effective objective MUST be `budget` and effective currency MUST be `usd`

### Requirement: Frequency Field

The system MUST accept an optional per-role `frequency` field (`high` | `medium` | `low`).
`frequency` MUST be read only when the role's effective `currency` is `quota` and MUST be
ignored (not an error) when the effective `currency` is `usd`. `frequency` is orthogonal to
`criticidad` — the two fields MUST NOT be conflated or defaulted from one another.

#### Scenario: Frequency ignored under usd currency

- GIVEN a role with `frequency: high` and effective `currency: usd`
- WHEN candidates are scored for the role
- THEN `frequency` MUST have no effect on the ranking

#### Scenario: Unknown frequency value rejected

- GIVEN a role with `frequency: extreme`
- WHEN the profile loads
- THEN loading MUST fail with a validation error naming the unknown frequency value

### Requirement: Budget Objective Requires an Effective Ceiling

The system MUST fail validation, fail-loud and at load time (consistent with Requirement:
Fail-Loud Validation), when a role's effective `objective` is `budget` and it has no
effective `max_input_price` — neither role-level nor inherited from `defaults`. A `budget`
role MUST NOT silently degrade to `value`-objective ranking for lack of a ceiling.

#### Scenario: budget objective without a price ceiling fails validation

- GIVEN a role with effective `selection.objective: budget` and no `max_input_price` set on
  either the role or `defaults`
- WHEN the profile is loaded
- THEN loading MUST fail with a validation error naming the missing `max_input_price` ceiling

#### Scenario: budget objective with a defaults-inherited ceiling loads

- GIVEN `defaults: { max_input_price: 5.0 }` and a role with effective
  `selection.objective: budget` that does not set its own `max_input_price`
- WHEN the profile is loaded
- THEN loading MUST succeed, and the role's effective ceiling MUST be the inherited `5.0`
