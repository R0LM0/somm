# API Client Specification (Kimi Key)

## Purpose

Define the optional Kimi API key field and how the setup wizard passes it to the API client.

## Requirements

### Requirement: Optional Kimi Key

The `api.Client` MUST store an optional `KIMAPIKey` string, populated from `NewClient`.

#### Scenario: Client initialized with Kimi key

- GIVEN `KIMI_API_KEY` is set to `"test-kimi"`
- WHEN `api.NewClient(nil, ocKey, orKey, kimiKey)` is called
- THEN `client.KIMAPIKey` MUST equal `"test-kimi"`

### Requirement: Environment Loading

The `main` package MUST load `KIMI_API_KEY` from `.env` and pass it to `api.NewClient`.

#### Scenario: `.env` contains Kimi key

- GIVEN `.env` contains `KIMI_API_KEY=secret`
- WHEN `somm` starts
- THEN the API client MUST have `KIMAPIKey="secret"`

### Requirement: Config Validation Status

`ValidateConfig` MUST report Kimi as configured when `KIMI_API_KEY` is non-empty.

#### Scenario: Kimi key present

- GIVEN `KIMAPIKey` is non-empty
- WHEN `ValidateConfig` runs
- THEN the provider list MUST include Kimi as configured
