# Proposal: Provider-Agnostic Role Profiles

## Intent

`somm`'s recommendation engine is hardcoded to the Gentle AI agent taxonomy
(`AllAgentRoles()`, 19 roles, Spanish `Criticidad`/`Criteria`, single `BestMetric`).
This makes it a personal tool: no other user (Claude Code, Cursor, custom orchestrators)
can use it on day one. Goal: make role definitions and scoring configurable via profiles,
so anyone brings their own roles, while Gentle AI becomes an embedded default preset with
byte-for-byte unchanged default output.

## Scope

### In Scope
- Replace hardcoded `AllAgentRoles()` with a `Profile` loaded from YAML: roles with
  `weights` over metrics and real hard constraints (`min_context`, `max_input_price`,
  `requires`, `exclude_family_of`).
- Upgrade scoring from single `BestMetric` to a normalized (0-1) weighted sum, applying
  hard constraints as pre-filters. Preserve the max-2 (relax-to-3) assignment-spreading
  rule and the quality/price ratio tiebreak.
- Ship current Gentle AI roles as an embedded `gentle-ai` default preset (behavior parity).
- Profile resolution order: flag -> `SOMM_PROFILE` env -> `./somm.yaml` -> XDG config -> built-in default.
- `get_agent_criteria` reads role descriptions from the active profile; Gentle AI markdown
  demoted to the `gentle-ai` preset's doc.

### Out of Scope (non-goals / future)
- `Catalog` provider-abstraction (Anthropic/OpenAI direct) — future change.
- `Exporter` target-abstraction (writing `.claude/settings.json`) — future change.
- New "speed" metric or new benchmark sources — data has no speed metric.

## Capabilities

### New Capabilities
- `role-profiles`: profile schema (roles, weights, hard constraints), loading, and
  flag/env/file/XDG/built-in resolution order.
- `weighted-scoring`: normalized weighted-sum scoring with hard-constraint pre-filters,
  preserving assignment-spreading and ratio tiebreak.
- `gentle-ai-preset`: embedded default preset reproducing today's 19-role output.

### Modified Capabilities
- None (no existing `openspec/specs/`; current behavior is unspecified).

## Approach

Introduce a `profile` package: YAML types + loader + resolver + embedded `gentle-ai.yaml`.
Rework `internal/api/recommend.go` to consume `Profile` roles (weights + constraints)
instead of `AllAgentRoles()`. Scoring normalizes each metric to 0-1 across candidates,
applies constraints as a pre-filter, computes weighted sum, then reuses the existing
spreading + ratio logic. `internal/guide` sources descriptions from the active profile.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/profile/` | New | Profile schema, loader, resolver, embedded `gentle-ai.yaml` |
| `internal/api/recommend.go` | Modified | Consume Profile; weighted scoring + constraint pre-filter |
| `internal/guide/guide.go` | Modified | Read descriptions from active profile |
| `cmd/server/main.go` | Modified | Wire profile resolution + `--profile` flag / env |
| `internal/api/recommend_test.go` | Modified | Golden parity for `gentle-ai` preset |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Default output regresses vs. today | High | Golden test locking `gentle-ai` preset to current output (KEY criterion) |
| Weighted sum shifts winners even with parity weights | Med | Preset uses single-weight-1.0 roles to reproduce `BestMetric` behavior exactly |
| Constraint pre-filter empties candidate set | Med | Fall back to relaxed/unconstrained pass; keep current no-model reason path |
| Internal API break for callers | Med | Contained refactor; update all `AllAgentRoles` callers + tests |

## Rollback Plan

Single contained change. Revert the `agnostic-role-profiles` commits; `AllAgentRoles()`
and `BestMetric` scoring return. No persisted state or migration involved.

## Dependencies

- A YAML library (e.g. `gopkg.in/yaml.v3`) added to `go.mod`.

## Success Criteria

- [ ] Default run (no profile) reproduces today's 19-role output byte-for-byte (golden).
- [ ] A custom YAML profile with its own roles/weights/constraints drives recommendations.
- [ ] Hard constraints (`min_context`, `max_input_price`, `requires`) filter candidates.
- [ ] Assignment-spreading (max-2, relax-3) and ratio tiebreak preserved.
- [ ] `get_agent_criteria` returns descriptions from the active profile.

## Proposal question round (assumptions pending user review)

Running non-interactively; the following assumptions were made and can be corrected or
expanded in a second round:
1. Parity is measured against current default output as the golden baseline (no scoring
   redesign for gentle-ai users beyond mapping `BestMetric` to weights).
2. `cheapest` semantics become a `max_input_price`/price-weighted expression in the preset,
   not a first-class metric.
3. Missing/`null` benchmark handling stays as today (skip candidate; `cheapest` fallback 50.0)
   under the new normalized scoring.
4. Invalid/malformed profile files should fail loud (error), not silently fall back to default.
