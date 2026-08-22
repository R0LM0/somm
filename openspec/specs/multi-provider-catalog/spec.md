# Multi-Provider Catalog Specification

## Purpose

Define optional local-CLI discovery of configured AI-provider subscriptions and their
live-priced models, the degradation contract when discovery is unavailable, and how
discovered models merge into the existing OC Go/Zen + OpenRouter catalog.

## Requirements

### Requirement: Discovery Is On By Default

The system MUST attempt local `opencode` CLI discovery (configured providers, live
per-model `cost.*`) by default, without requiring explicit opt-in, matching the existing
OpenRouter-enrichment precedent (see Requirement: Graceful Degradation for failure
handling).

#### Scenario: Discovery runs without an extra flag

- GIVEN a host with `opencode` installed and authenticated, and no discovery-specific
  configuration set
- WHEN `list_available_models` or `recommend_config` runs
- THEN discovered provider models MUST appear in the returned catalog automatically

### Requirement: Graceful Degradation

The system MUST warn (never fail) and continue serving today's OC Go/Zen + OpenRouter
catalog when the `opencode` CLI is absent, unauthenticated, returns malformed output, or
exceeds its timeout. `list_available_models` and `recommend_config` MUST NOT error or
return an empty result solely because discovery failed.

#### Scenario: Binary absent

- GIVEN `opencode` is not on PATH
- WHEN `list_available_models` runs
- THEN the call MUST succeed and return today's OC Go/Zen + OpenRouter catalog unchanged

#### Scenario: Unauthenticated, malformed, or slow discovery

- GIVEN `opencode auth list` reports no authenticated providers, or `opencode models
  --verbose` returns malformed JSON, or the CLI exceeds its allotted time
- WHEN discovery runs
- THEN the system MUST log a warning and continue with the existing catalog, and MUST NOT
  fail `list_available_models` or `recommend_config`

### Requirement: Discovered Models Carry Provider Identity

Each model discovered via the CLI MUST be tagged with the provider identity reported by
`opencode models --verbose` (`providerID`), distinguishing it from OC Go/Zen-sourced
models.

#### Scenario: Four configured providers are tagged correctly

- GIVEN `opencode auth list` reports 4 configured providers (OpenAI oauth, Kimi For
  Coding api, OpenCode Go api, GitHub Copilot oauth)
- WHEN models are discovered
- THEN every returned catalog entry MUST carry the `providerID` of the provider it came
  from

### Requirement: Merge Into Existing Catalog

Discovered models MUST be merged into the same catalog served by `list_available_models`
and consumed by `recommend_config`, alongside OC Go/Zen + OpenRouter-sourced models,
without requiring a separate tool call.

#### Scenario: Single catalog response includes both sources

- GIVEN `opencode` discovery succeeds and OC Go/Zen fetch succeeds
- WHEN `list_available_models` runs
- THEN the response MUST contain both discovered-provider models and OC Go/Zen models in
  one merged list

### Requirement: CLI Price Wins On Duplicate Models

When the same model is reachable via both the `opencode` CLI and the OC Go/Zen +
OpenRouter path, the CLI's first-party `cost.*` price MUST take precedence over the
OpenRouter-derived price in the merged entry.

#### Scenario: Same model priced differently by each source

- GIVEN model X priced via OpenRouter alias-matching at $3.00/$15.00 per 1M tokens and
  discovered via `opencode models --verbose` with `cost.input: 2.50, cost.output: 12.00`
- WHEN the catalog is merged
- THEN the merged entry for X MUST report $2.50/$12.00, not the OpenRouter-derived price

### Requirement: Zero-Price Discovered Models Are Excluded With a Distinguishable Reason

A discovered model whose `cost.input == 0` and `cost.output == 0` (flat-rate,
OAuth-subscription-gated, no per-token billing) MUST be excluded from ranking, consistent
with the existing zero/nil-price exclusion `collectCandidates` already applies. The
exclusion MUST be explainable: the system MUST surface an explicit reason
("flat-rate subscription, no usage-cap ranking available") distinguishable from the
generic reason used for models that simply lack pricing data.

#### Scenario: OAuth-gated $0 models are annotated, not silently dropped

- GIVEN `opencode models --verbose` reports `cost.input: 0, cost.output: 0` for the
  OpenAI oauth and GitHub Copilot oauth models
- WHEN `list_available_models` returns the catalog
- THEN both models MUST be present and annotated with the flat-rate exclusion reason,
  distinguishable from any model excluded for having no pricing data at all

#### Scenario: A $0-price model never wins a role, and the role reports the fallback reason

- GIVEN a role whose only constraint-filtered candidates are $0-price discovered models
- WHEN `recommend_config` computes that role's recommendation
- THEN the role MUST resolve through the existing "no model available" reason path
  (weighted-scoring, Requirement: Empty Candidate Set Fallback), never selecting a
  $0-price model as the winner

### Requirement: Behavior Unchanged When Discovery Is Absent

When discovery yields zero providers, `list_available_models` and `recommend_config`
output MUST be byte-for-byte identical to today's OC Go/Zen + OpenRouter-only behavior.

#### Scenario: No discovered providers changes nothing

- GIVEN `opencode` is absent or fully unauthenticated
- WHEN `list_available_models` and `recommend_config` run
- THEN their output MUST match pre-change behavior exactly
