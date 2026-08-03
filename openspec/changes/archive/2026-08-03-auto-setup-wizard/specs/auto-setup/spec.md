# Auto-Setup Specification

## Purpose

Define when and how `somm` automatically launches the setup wizard instead of starting the MCP server.

## Requirements

### Requirement: Pre-flight Config Check

The system MUST determine whether a required OpenCode API key is available before starting the MCP server. A key is considered available if the environment variable `OPENCODE_API_KEY` is non-empty after loading `.env` from the binary directory and the current directory.

#### Scenario: Missing key in interactive terminal launches wizard

- GIVEN `OPENCODE_API_KEY` is not set in environment or `.env`
- AND the process stdin is a terminal
- WHEN the binary is invoked without the `setup` subcommand
- THEN the setup wizard MUST run before attempting to start the server

#### Scenario: Missing key in non-interactive terminal exits with error

- GIVEN `OPENCODE_API_KEY` is not set
- AND the process stdin is not a terminal
- WHEN the binary is invoked without the `setup` subcommand
- THEN the system MUST print an error message instructing the user to run `somm setup` or set the key
- AND exit with a non-zero status

### Requirement: Skip-Setup Flag

The system MUST support a `--skip-setup` CLI flag that bypasses auto-launch and behaves as before: if the key is missing, the server returns a fatal error.

#### Scenario: `--skip-setup` preserves old failure behavior

- GIVEN `--skip-setup` is passed
- AND `OPENCODE_API_KEY` is missing
- WHEN the binary starts
- THEN the system MUST return the existing `-opencode-api-key is required` error
- AND MUST NOT launch the setup wizard

### Requirement: Re-evaluate After Wizard

When the wizard is launched automatically, the system MUST re-check the config after it returns. If the key is still missing, the system MUST exit with a non-zero status.

#### Scenario: User cancels wizard

- GIVEN auto-setup was triggered
- AND the user cancels the wizard without providing a key
- WHEN the wizard returns
- THEN the system MUST exit with a non-zero status
