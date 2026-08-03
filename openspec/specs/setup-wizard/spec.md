# Setup Wizard Specification

## Purpose

Define the interactive setup flow for first-time and reconfiguration runs, including provider selection, key prompts, and persistence.

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

### Requirement: Persistence

After collecting keys, the wizard MUST write a `.env` file next to the resolved binary and update `opencode.json` with the `somm` MCP entry.

#### Scenario: Save writes keys and config

- GIVEN keys are collected for OpenCode and OpenRouter
- WHEN the wizard saves
- THEN `.env` MUST contain `OPENCODE_API_KEY` and `OPENROUTER_API_KEY`
- AND `opencode.json` MCP.somm MUST contain `command` equal to the binary path, `enabled` true, and `type` `local`

### Requirement: Reconfiguration with `--force`

Passing `--force` to `somm setup` MUST bypass status mode and run the full provider/key flow.

#### Scenario: `--force` reconfigures existing installation

- GIVEN `.env` and `opencode.json` are already configured
- WHEN `somm setup --force` runs
- THEN the wizard MUST prompt for providers and keys again
