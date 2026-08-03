# Gentle AI Preset Specification

## Purpose

Define the embedded default profile that reproduces today's hardcoded 19-role Gentle AI
output byte-for-byte, ensuring existing users see no behavior change when no external
profile is configured.

## Requirements

### Requirement: Embedded Default Preset Parity

The system MUST embed a `gentle-ai` preset (via `//go:embed`) containing all 19 roles from
today's `AllAgentRoles()`, with each role's former single `BestMetric` expressed as a
`weights` map containing exactly one metric key at weight `1.0`, and each role's Spanish
`Criteria` text preserved as `description`. When no `--profile`, `SOMM_PROFILE`,
`./somm.yaml`, or XDG config file is present, this preset MUST be the active profile.

#### Scenario: Default run reproduces pre-refactor output byte-for-byte

- GIVEN no profile flag, env var, `./somm.yaml`, or XDG config file is present
- AND a fixed set of mock OpenRouter/OpenCode candidate models identical to the golden
  fixture's inputs
- WHEN a recommendation run executes with the embedded `gentle-ai` preset
- THEN `FormatRecommendations` output MUST match the checked-in `testdata/gentle-ai.golden`
  fixture byte-for-byte

#### Scenario: Single-weight roles bypass normalization

- GIVEN every role in the `gentle-ai` preset has exactly one weight at `1.0`
- WHEN scoring runs for any `gentle-ai` role
- THEN normalization MUST never engage (per weighted-scoring's multi-metric requirement) and
  `Qraw` MUST equal the role's single raw benchmark value

### Requirement: Cheapest Roles Map to Price-Minimizing Objective

The system MUST map every role that was previously driven by the `cheapest` `BestMetric` to
an empty `weights: {}` map in the `gentle-ai` preset. A role with empty `weights` MUST be
scored by sorting candidates by price ascending, with intelligence descending as tiebreak,
using `50.0` as the intelligence value when a candidate's intelligence benchmark is `null`.

#### Scenario: Cheapest role sorts by price ascending

- GIVEN a `gentle-ai` role previously using `cheapest` `BestMetric`, mapped to `weights: {}`
- AND candidates with distinct prices
- WHEN scoring runs for that role
- THEN the lowest-price candidate satisfying hard constraints MUST be selected first

#### Scenario: Null intelligence falls back to 50.0 for tiebreak

- GIVEN two candidates tied on price, one with `intelligence == null` and one with
  `intelligence == 40.0`
- WHEN the price-minimizing role breaks the tie
- THEN the `null` candidate MUST be treated as `intelligence == 50.0` and MUST win the
  tiebreak over the candidate with `intelligence == 40.0`
