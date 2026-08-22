# Delta for weighted-scoring

## MODIFIED Requirements

### Requirement: Hard Constraint Pre-Filter

For each role, the system MUST filter candidates, per assignment attempt, in this order:
(1) skip models with no usable pricing, (2) skip models already at the per-model
assignment cap, (3) apply hard constraints `min_context`, `max_input_price`, `requires`,
`exclude_family_of`, and provider scope (role-profiles, Requirement: Selection Block
Schema) — a candidate whose provider identity is not in the role's effective, non-empty
`providers` set MUST be excluded, (4) for weighted roles, skip candidates with a `nil`
value on any positively-weighted metric. Provider scope acts as a hard pre-filter, the
same tier as `min_context`/`max_input_price`/`requires`/`exclude_family_of` — never a soft
ranking factor. Hard constraints MUST NOT be relaxed at any point.

(Previously: hard constraints were `min_context`, `max_input_price`, `requires`, and
`exclude_family_of` only — no provider scope.)

#### Scenario: `min_context` and `max_input_price` filter candidates

- GIVEN a role with `min_context: 100000` and `max_input_price: 5.0`, and candidates with
  context lengths and prices both above and below those bounds
- WHEN candidates are filtered for the role
- THEN only candidates meeting both constraints MUST remain in the candidate set

#### Scenario: `exclude_family_of` excludes a family already assigned

- GIVEN role `B` has `exclude_family_of: A`, and role `A` was already assigned a model
  whose family is `openai`
- WHEN candidates are filtered for role `B`
- THEN candidates whose family is `openai` MUST be excluded from role `B`'s candidate set

#### Scenario: Provider scope excludes out-of-scope candidates

- GIVEN a role with effective `providers: ["OpenCode Go"]` and candidates from providers
  OpenCode Go, OpenAI, and Kimi For Coding
- WHEN candidates are filtered for the role
- THEN only the OpenCode Go candidate(s) MUST remain in the candidate set

### Requirement: Empty Candidate Set Fallback

When a role's constraint-filtered candidate set is empty at the current assignment cap,
the system MUST attempt the existing 2-to-3 assignment-cap relaxation before treating the
role as unassignable. Hard constraints MUST remain applied during relaxation. If the
candidate set is still empty after relaxation, the system MUST report the role using
today's per-role "no model available" reason path rather than raising a hard error or
panicking.

(Previously: emptiness could result from `min_context`/`max_input_price`/`requires`/
`exclude_family_of` only; provider scope is now also a possible cause.)

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

#### Scenario: No configured provider satisfies the role's scope

- GIVEN a role with effective `providers: ["Anthropic"]` and no currently configured or
  discovered provider matches "Anthropic"
- WHEN candidates are collected at both assignment caps
- THEN the candidate set MUST remain empty after relaxation, the system MUST NOT panic or
  raise a fatal error, and the role MUST surface the existing "no model available" reason
