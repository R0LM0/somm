# Delta for role-profiles

## MODIFIED Requirements

### Requirement: Selection Block Schema

The system MUST accept an optional `selection` block, at the profile level and/or
per-role, with three independent fields: `objective` (`value` | `quality` | `budget`,
default `value`), `currency` (`usd` | `quota`, default `usd`), and `providers` (a list of
provider identity tokens, default unset/empty meaning "all configured providers" — see
Requirement: Provider Scope Default Is All Configured Providers). An unrecognized
`objective` or `currency` value MUST fail validation. Each `providers` entry MUST be a
non-empty, non-whitespace-only string; a syntactically malformed entry MUST fail
validation, but a `providers` entry naming a provider that is not currently configured or
discovered MUST NOT fail validation — that resolves to zero eligible candidates at ranking
time instead (weighted-scoring, Requirement: Hard Constraint Pre-Filter). When `selection`
is absent entirely, resolved behavior MUST be byte-for-byte identical to a profile with no
`selection` block.

(Previously: schema had only two independent fields, `objective` and `currency`; no
`providers` field existed.)

#### Scenario: Default profile has no selection block

- GIVEN a profile with no `selection` key anywhere
- WHEN the profile loads
- THEN the effective objective MUST be `value`, effective currency MUST be `usd`, and
  effective provider scope MUST be "all configured providers" for every role

#### Scenario: Unknown objective value rejected

- GIVEN `selection: { objective: cheapest }`
- WHEN the profile loads
- THEN loading MUST fail with a validation error naming the unknown objective value

#### Scenario: Malformed providers entry rejected

- GIVEN `selection: { providers: [""] }`
- WHEN the profile loads
- THEN loading MUST fail with a validation error naming the malformed `providers` entry

#### Scenario: Unconfigured provider name does not fail validation

- GIVEN `selection: { providers: ["Anthropic"] }` and "Anthropic" is not currently a
  configured or discovered provider
- WHEN the profile loads
- THEN loading MUST succeed

### Requirement: Per-Role Selection Override

Per decision D3, a role-level `selection` block MUST override the profile-level
`selection` block field-by-field: a role that sets only one of `objective`, `currency`, or
`providers` MUST inherit the profile-level value of the other two.

(Previously: field-by-field override applied to two fields, `objective` and `currency`
only.)

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
- THEN the role's effective objective MUST be `budget` and effective currency MUST be
  `usd`

#### Scenario: Role overrides only providers, inherits objective and currency

- GIVEN profile-level `selection: { objective: quality, currency: usd }` and a role with
  `selection: { providers: ["OpenCode Go", "Kimi For Coding"] }`
- WHEN the role resolves its effective selection
- THEN the role's effective providers MUST be `["OpenCode Go", "Kimi For Coding"]`, and
  its effective objective and currency MUST be inherited unchanged (`quality`, `usd`)

## ADDED Requirements

### Requirement: Provider Scope Default Is All Configured Providers

Per decision D3, when a role's effective `providers` field is unset or empty, the role
MUST rank against all currently configured providers. `providers` is an opt-in narrowing
of the candidate set, never an opt-in inclusion requirement.

#### Scenario: Unscoped role sees every configured provider

- GIVEN no `providers` field set at profile or role level, and 4 configured providers
  (OpenAI oauth, Kimi For Coding api, OpenCode Go api, GitHub Copilot oauth)
- WHEN the role's candidates are collected
- THEN candidates from all 4 providers MUST be eligible, not only OpenCode Go/Zen
