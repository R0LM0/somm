# Design: Provider-Agnostic Role Profiles

## Technical Approach

Introduce a standalone `internal/profile` package (YAML schema + loader + resolver + embedded
`gentle-ai` preset). Rework `internal/api/recommend.go` to consume `[]profile.Role` (weights +
hard constraints) instead of `AllAgentRoles()`. `cmd/somm/main.go` resolves the active `Profile`
and threads it into `RecommendConfig` and `get_agent_criteria`. No import cycle: `profile` has no
`api` dependency; `api` imports `profile`.

## Architecture Decisions

### Decision 1 (LOAD-BEARING): raw weighted sum feeds the quality/price ratio — never a normalized value

Today `findBestModel` ranks by `ratio = rawBenchmark / price` (recommend.go:207), tiebreak raw
score desc. Min-max normalization is an **affine shift** `norm = (v−min)/range`; dividing a shifted
value by price is **not** order-preserving vs. dividing the raw value by price, so normalize-then-ratio
reorders winners.

**Worked example** — A: intel=72.5, price=30; B: intel=52.1, price=0.95.
- Today (raw ratio): A=2.417, B=54.8 → **B wins**.
- Normalize-then-ratio (min=52.1, range=20.4): normA=1.0, normB=0.0 → A=0.033, B=0.0 → **A wins**. Parity broken.

**Choice**: the quality figure used in the ratio is the **raw weighted sum** `Qraw = Σ wₘ·rawₘ`
(benchmarks share a native ~0–100 scale, so a raw weighted sum is already commensurable). Ratio =
`Qraw / price`, tiebreak `Qraw` desc — identical shape to today. For the gentle-ai preset every role
has exactly one weight = 1.0, so `Qraw = rawBenchmark` and the ratio is byte-for-byte today's.

**Rejected**: normalize-then-ratio (worked example above); normalized weighted sum as the sole sort
key discarding the price ratio (drops today's calidad/precio behavior).

### Decision 2: normalization is defined but engages ONLY for multi-metric roles

`normₘ = (rawₘ − minₘ)/(maxₘ − minₘ)`, per-role, over that role's **constraint-filtered candidate
set**. Engages only when a role weights ≥2 metrics (custom profiles). Edge cases: `maxₘ==minₘ`
(range 0, incl. single candidate) → `normₘ = 1.0` (neutral, no div-by-zero). The gentle-ai preset is
all single-weight, so normalization never fires for it → parity is unconditional. `Qnorm` is used as
the ratio numerator **only** for multi-metric roles; single-metric roles always use `Qraw`.

### Decision 3: `cheapest` → empty `weights` = price-minimizing objective

Preset maps the old `cheapest` roles to `weights: {}`. Empty weights ⇒ scorer sorts by price asc,
tiebreak intelligence desc with the **`50.0` fallback** when intelligence is null — reproducing
recommend.go:188–195 and 261–266 exactly. `max_input_price` is available as a hard cap on top.

### Decision 4: null-benchmark handling preserved (endorsed assumption 3)

Weighted role: a candidate with a null value on any positively-weighted metric is **skipped**
(mirrors today's `continue`). Price-minimizing role: candidates are never skipped for null
benchmarks; intelligence tiebreak uses `50.0` fallback.

### Decision 5: constraint pre-filter order + relaxation (endorsed assumption on fail-soft)

Per role, per assignment cap:
1. Skip models with no pricing (`Pricing==nil || Prompt==0`) — as today.
2. Skip models at the assignment cap.
3. Apply hard constraints: `min_context` (`ContextLength ≥ min`), `max_input_price` (`price ≤ max`),
   `requires` (capability tokens present), `exclude_family_of` (family ≠ excluded).
4. Weighted roles: skip candidates with a null positively-weighted metric.

**Relaxation**: only the assignment cap relaxes 2→3 (recommend.go:178–182). Hard constraints are
**never** relaxed. Still empty → today's per-role no-model reason path (soft, not a hard error).
`family := deriveProvider(ocID)` (reuses provider.go prefix logic). `exclude_family_of: <role-id>`
excludes models whose family equals the family already assigned to `<role-id>` this run; today it is
only decorative text (jd-judge-b), so the preset leaves it empty → parity.

### Decision 6: malformed profile FAILS LOUD (endorsed assumption 4)

Any file selected by the resolution order that is malformed or fails schema validation (unknown
metric key, bad YAML, missing `id`) → `Resolve` returns an error → `run()` returns fatal; the server
does not start. No silent fallback to embedded. The embedded default is always valid; "no file found"
is not an error (uses embedded).

## Data Flow

    --profile / SOMM_PROFILE / ./somm.yaml / XDG / embedded
              │  profile.Resolve() → *Profile (or fatal error)
              ▼
    cmd/somm/main.go ──► RecommendConfig(ctx, client, prof, roleFilter)
              │                    │ per role: pre-filter → Qraw/Qnorm → ratio
              │                    ▼
              └──► registerGetAgentCriteria(server, prof) ──► role.Description

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/profile/profile.go` | Create | `Profile`, `Role`, `RoleDefaults` types; metric keys |
| `internal/profile/load.go` | Create | `gopkg.in/yaml.v3` parse + validation (fail loud) + defaults merge |
| `internal/profile/resolve.go` | Create | flag→env→`./somm.yaml`→XDG→embedded order |
| `internal/profile/preset.go` | Create | `//go:embed presets/gentle-ai.yaml` embedded default |
| `internal/profile/presets/gentle-ai.yaml` | Create | 19 roles; `BestMetric`→weights; Criteria→description |
| `internal/api/recommend.go` | Modify | Consume `[]profile.Role`; `Qraw` ratio + constraint pre-filter; `buildReason` shows dominant-weight metric |
| `internal/guide/guide.go` | Modify | `guia_gentle_ai.md` demoted to gentle-ai preset doc; descriptions sourced from Profile |
| `cmd/somm/main.go` | Modify | `--profile` flag/env; `profile.Resolve`; thread `*Profile` into 8 callers |
| `internal/api/{cost,export,validate}.go` | Modify | Pass `*Profile` through updated `RecommendConfig` signature |
| `internal/api/recommend_test.go` | Modify | Add golden parity test |
| `go.mod` | Modify | Add `gopkg.in/yaml.v3` |

## Interfaces / Contracts

```go
type Profile struct {
    Version  int
    Defaults RoleDefaults
    Roles    []Role
}
type Role struct {
    ID, Description, Criticidad string
    Weights         map[string]float64 // keys: intelligence|coding|agentic (NO speed)
    MinContext      *int64
    MaxInputPrice   *float64
    Requires        []string // capability tokens, e.g. "reasoning"
    ExcludeFamilyOf string
}
// Signature change (8 callers):
func RecommendConfig(ctx, client, prof *profile.Profile, roleFilter []string) (...)
```

Defaults merge: role-level nil `min_context`/`max_input_price`/`requires` inherit `Defaults`;
`weights` is per-role. **MCP contract stable**: `get_agent_criteria` and `recommend_config` keep the
same tool name, input schema, and text-output shape.

## Testing Strategy

| Layer | What | Approach |
|-------|------|----------|
| Golden (KEY) | gentle-ai preset output unchanged | Fixed OC+OR mock models → `FormatRecommendations` vs. checked-in `testdata/gentle-ai.golden` captured from pre-refactor output; byte-for-byte |
| Unit — loader | parse, unknown-metric error, malformed error, defaults merge | table tests |
| Unit — scoring | `Qraw` ratio worked example; constraint pre-filter; 2→3 relaxation; empty-weights cheapest incl. `50.0`; normalization range-0 & single-candidate | table tests |
| Unit — resolve | flag>env>file>XDG>embedded; malformed selected file → error | env + tmpfile |

## Threat Matrix

N/A — no routing, shell, subprocess, VCS/PR automation, executable-file classification, or
process-integration boundary. The only new surface is a user-pointed YAML **file read** (flag/env/
XDG); the user already controls that path, so no traversal/privilege concern beyond ordinary config
loading.

## Migration / Rollout

No data migration. Behavior parity guaranteed by the golden test. `guia_gentle_ai.md` moves to the
gentle-ai preset doc; no user-facing tool contract change. Rollback = revert commits (`AllAgentRoles`
returns).

**Review budget**: authored additions+deletions ≈ 520 lines (goldens excluded) → **400-line risk:
High**. MUST chain into 2 slices:
- **Slice 1**: `internal/profile` package (schema, loader, resolver, embedded gentle-ai preset) +
  unit tests. Standalone, no behavior change.
- **Slice 2**: rework `recommend.go`, wire `main.go`/`guide`, golden parity test.

## Open Questions

- [ ] `requires` initial token set: start with `{"reasoning"}` (model.Reasoning != nil); expand later.
