# Delta for Setup Wizard

## MODIFIED Requirements

### Requirement: Persistence

After collecting keys and, per decision D1, the OpenCode tier (see Requirement: Tier
Capture), the wizard MUST write a `.env` file next to the resolved binary and update
`opencode.json` with the `somm` MCP entry.

(Previously: `.env` persisted only `OPENCODE_API_KEY` and `OPENROUTER_API_KEY`; there was no
tier capture step and no `SOMM_OC_TIER` key.)

#### Scenario: Save writes keys and config

- GIVEN keys are collected for OpenCode and OpenRouter
- WHEN the wizard saves
- THEN `.env` MUST contain `OPENCODE_API_KEY` and `OPENROUTER_API_KEY`
- AND `opencode.json` MCP.somm MUST contain `command` equal to the binary path, `enabled`
  true, and `type` `local`

#### Scenario: Save writes the persisted tier

- GIVEN the tier capture step (Requirement: Tier Capture) collected `go`
- WHEN the wizard saves
- THEN `.env` MUST contain `SOMM_OC_TIER=go` alongside `OPENCODE_API_KEY` and
  `OPENROUTER_API_KEY`

## ADDED Requirements

### Requirement: Tier Capture

Per decision D1, the wizard MUST insert a new blocking screen between "Key Prompts" and
"Persistence" that asks the user which OpenCode tier they are on (`go` | `zen`), MUST persist
the answer as `SOMM_OC_TIER` in `.env` via the existing `saveEnvFile` path, and MUST NOT
re-ask on any later run once `.env` already contains `SOMM_OC_TIER`, unless `--force` is
passed (Requirement: Reconfiguration with `--force`). This question MUST NOT be inferred from
the `Subscription: go|zen|both` catalog stamp — that stamp reports which catalog a model
appears in, not which tier the user pays for, and a bad guess would silently misrank every
recommendation.

#### Scenario: Tier is asked once on first setup

- GIVEN `.env` does not yet contain `SOMM_OC_TIER`
- WHEN `somm setup` runs through the key-prompt step
- THEN the wizard MUST show the tier-capture screen before the persistence step and block
  until the user answers `go` or `zen`

#### Scenario: Tier is not re-asked on a later run

- GIVEN `.env` already contains `SOMM_OC_TIER=zen`
- WHEN `somm setup` runs again without `--force`
- THEN the wizard MUST skip the tier-capture screen and MUST NOT overwrite the persisted
  value

#### Scenario: `--force` re-asks the tier

- GIVEN `.env` already contains `SOMM_OC_TIER=go`
- WHEN `somm setup --force` runs
- THEN the wizard MUST show the tier-capture screen again and MAY overwrite `SOMM_OC_TIER`
  with the new answer
