# Setup Wizard Specification

## Purpose

Define the interactive setup flow for first-time and reconfiguration runs, including provider selection, key prompts, tier capture, and persistence.

## Requirements

### Requirement: Status Mode

If the system detects that `.env` contains the required `OPENCODE_API_KEY` and `opencode.json` already contains a valid `somm` MCP entry, the wizard MUST enter status mode and skip key entry, unless `--force` is passed.

#### Scenario: Already configured wizard shows status

- GIVEN `.env` contains a non-empty `OPENCODE_API_KEY`
- AND `opencode.json` contains a `somm` MCP entry with a non-empty command
- WHEN `somm setup` runs without `--force`
- THEN the wizard MUST print the current status and exit without prompting

### Requirement: Provider Selection

When not in status mode, the wizard MUST ask the user which providers they have using a multi-select. The options MUST be:

- OpenCode Go/Zen (required and pre-selected)
- OpenRouter (optional)

The user MUST NOT be able to deselect OpenCode.

#### Scenario: User selects all providers

- GIVEN the wizard is not in status mode
- WHEN the user selects OpenCode and OpenRouter
- THEN the system MUST prompt for an API key for each selected provider

#### Scenario: OpenCode is required

- GIVEN the user deselects every provider including OpenCode
- WHEN the selection is submitted
- THEN the system MUST reject the submission and require OpenCode

### Requirement: Key Prompts

For each selected provider, the wizard MUST prompt for the API key. The OpenCode key MUST be non-empty. The OpenRouter key MAY be empty if the user leaves it blank.

#### Scenario: Required OpenCode key rejected if empty

- GIVEN the wizard prompts for the OpenCode API key
- WHEN the user submits an empty value
- THEN the system MUST re-prompt or prevent submission

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

### Requirement: Reconfiguration with `--force`

Passing `--force` to `somm setup` MUST bypass status mode and run the full provider/key flow, including the tier-capture screen.

#### Scenario: `--force` reconfigures existing installation

- GIVEN `.env` and `opencode.json` are already configured
- WHEN `somm setup --force` runs
- THEN the wizard MUST prompt for providers, keys, and tier again
