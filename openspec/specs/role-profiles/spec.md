# Role Profiles Specification

## Purpose

Define the YAML profile schema that replaces the hardcoded `AllAgentRoles()` taxonomy,
the defaults-merge rule between role-level and profile-level fields, and the deterministic
resolution order used to select the active profile at startup.

## Requirements

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

### Requirement: Capability Token Support

The system MUST support at least the `reasoning` capability token in `requires`, matched
against a candidate model when `model.Reasoning != nil`. The token set MAY be expanded in
future changes without breaking existing profiles.

#### Scenario: `requires: ["reasoning"]` filters non-reasoning models

- GIVEN a role with `requires: ["reasoning"]`
- WHEN candidates are filtered and a candidate model has `Reasoning == nil`
- THEN that candidate MUST be excluded from the role's candidate set

### Requirement: Defaults Merge

The system MUST merge `defaults.min_context`, `defaults.max_input_price`, and
`defaults.requires` into any role that leaves the corresponding field `nil`/unset. `weights`
MUST NOT be defaulted — every role's `weights` are role-specific only.

#### Scenario: Role inherits default hard constraints

- GIVEN `defaults: { min_context: 8000 }` and a role that does not set `min_context`
- WHEN the profile is loaded
- THEN the role's effective `min_context` MUST be `8000`

#### Scenario: Role overrides a default

- GIVEN `defaults: { min_context: 8000 }` and a role that sets `min_context: 32000`
- WHEN the profile is loaded
- THEN the role's effective `min_context` MUST be `32000`

#### Scenario: Weights are never defaulted

- GIVEN `defaults` includes no `weights` field (weights cannot be declared at defaults level)
- WHEN a role omits `weights`
- THEN the role's effective `weights` MUST be empty, not inherited from any other role

### Requirement: Resolution Order

The system MUST resolve the active profile in this exact precedence order, using the first
source that is present: (1) `--profile` CLI flag path, (2) `SOMM_PROFILE` environment
variable path, (3) `./somm.yaml` in the current working directory, (4) an XDG config path,
(5) the embedded `gentle-ai` default preset.

#### Scenario: Flag wins over env and file

- GIVEN `SOMM_PROFILE` is set, `./somm.yaml` exists, and `--profile custom.yaml` is passed
- WHEN the profile resolves
- THEN `custom.yaml` MUST be loaded

#### Scenario: No external source present falls through to embedded default

- GIVEN no flag, no `SOMM_PROFILE`, no `./somm.yaml`, and no XDG config file
- WHEN the profile resolves
- THEN the embedded `gentle-ai` preset MUST be loaded, and this MUST NOT be treated as an error

### Requirement: Fail-Loud Validation

The system MUST return a fatal error and MUST NOT start the server when the profile file
selected by the resolution order is malformed YAML, fails schema validation (unknown metric
key, missing `id`), or otherwise cannot be parsed. The system MUST NOT silently fall back to
the embedded default when an explicitly selected file is invalid.

#### Scenario: Malformed profile fails loud, no silent fallback

- GIVEN `./somm.yaml` exists and contains invalid YAML syntax
- WHEN the server starts and resolves the profile
- THEN `Resolve` MUST return an error, `run()` MUST return fatal, and the server MUST NOT
  start using the embedded default instead

#### Scenario: Missing `id` on a role fails validation

- GIVEN a profile file with a role entry that omits `id`
- WHEN the profile is loaded
- THEN loading MUST fail with a validation error identifying the role missing `id`

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
