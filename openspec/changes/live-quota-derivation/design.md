# Design: Derive Plan Quota at Request Time from Live Pricing

## Technical Approach

`internal/profile/plans` stops storing a volatile request count and stores curated
*shape + tier + multiplier*. It gains a price-aware derivation:

```
cost_per_request = (in*P_in + cached*P_cacheRead + out*P_out) / 1e6
requests(window) = multiplier * (tier_usd / windowDivisor) / cost_per_request
```

`internal/api` owns price acquisition and projects it into a `plans.Price` value.
`plans` never imports `api`; `Price` is the seam. `resolveQuotaDenominators` keeps
consuming a **5h-scale** number, so `hSat = 200` and the spec's Frequency Weighting
requirement are untouched.

## Architecture Decisions

### D1 — Window divisors are exactly 5 / 2 / 1

**Choice**: `budget_usd = tier_usd / 5` (5h), `/ 2` (week), `/ 1` (month). Go constants,
not YAML. Derive all three; consume 5h.

**Alternatives**: (a) per-model approximate divisors; (b) derive month only, keep 5h/week curated.

**Rationale**: the proposal's "4.45–5.06 range" is a *display-rounding artifact*, not
real variance. Undoing 2–3 sig-fig rounding, `month/5h == 5.0` and `month/week == 2.0`
hold for 20 of 22 published rows (GLM-5.3: 1080/5 = 216, displayed "220"; Qwen3.8:
810/5 = 162, displayed "160"). Only two rows resist: GLM-5.2/5.1 (2.3%, from scaling
GLM-5.3's already-rounded 220 by the 4× tier ratio) and Kimi K3 (11%, unexplained).
No curated constant survives; (b) is strictly dominated. Verified: Grok 5h =
(15/5)/0.02497 = 120.1 vs published 120; Hy3 = (60/5)/0.0027898 = 4301 vs 4300.

**hSat consequence**: derived 5h values stay on the same scale as today's table
(Grok 120, Kimi K3 98, Qwen3.8 162, GLM-5.3 216 differentiate; cheap models saturate).
`hSat = 200` needs **no** recalibration. Choosing (b)'s month input would have forced
either an inline `/5` or an hSat rewrite the proposal scoped out.

### D2 — OpenRouter price proxy accepted as documented risk

**Choice**: accept, with mitigations. **The spike was not performed — this session has
no Bash, shell, or network tool.** Stated plainly rather than assumed away.

**Rationale + mitigations**: (1) blast radius is bounded — quota only differentiates
below ~200 req/5h, and every model in that band is already the expensive tail, so a
proxy error moves rankings within an already-last cohort; (2) all price acquisition is
isolated in one `priceOf(EnrichedModel) plans.Price` function, making a future OC-native
source a one-function swap; (3) `buildReason` says "derivado del precio actual", never
"medido". A raw-body inspection stays a non-blocking follow-up task.

### D3 — Muse Spark is filtered upstream, not on the fallback bridge

**Choice**: no change. No alias, no hardcoded price.

**Rationale**: the premise is wrong. `collectCandidates:352` skips `m.Pricing == nil`
**before** `resolveQuotaDenominators` runs, so a curated-but-priceless model never
reaches the `price/P_min` bridge at all — it is excluded from candidacy exactly like
Ox Alpha Free. Nothing needs accepting. Ranking under *either* currency requires a
price (usd uses it directly, quota needs it for `cost_per_request`), and a hardcoded
fallback price would reintroduce the exact staleness this change removes. Needs one
RED test locking "curated + nil price → absent from candidates, no panic".

**MiniMax M2.5** (priced, no published shape): stays table-absent → existing `price/P_min`
bridge, unchanged. Do not inherit M2.7's shape — that fabricates curation data.

### D4 — Always the cheaper variant (off-peak / lower context tier)

**Choice**: assume off-peak and the ≤256K/≤272K tier. No detection.

**Rationale**: (1) the ≤tier price is *correct*, not conventional — every curated shape
totals ~57–88K tokens, far under 256K; (2) the reference snapshot validated to ~0% using
off-peak, and mixing peak prices with off-peak-calibrated shapes yields an incoherent
number; (3) detection is impossible — OpenRouter exposes one number with no variant
field, so "detect" would be a guess dressed as logic; (4) **ranking-inert today**: all
six affected models derive 1050–7600 req/5h off-peak and 525–3800 at 2× price — both
far above `hSat = 200`, so both variants saturate identically.

### D5 — `cache_read_ratio` fallback (new; not in the proposal)

**Choice**: curate `cache_read_ratio` (cache-read ÷ input price) per model. Use live
`input_cache_read` when OpenRouter reports it, else `InputPerM * ratio`.

**Rationale**: the cached term dominates cost (86% for Grok). `Pricing.InputCacheRead`
is `*string` — optional. Silently dropping it would understate cost ~7× and overstate
quota by the same factor. The ratio spans 0.008–0.25 across models, so no global
constant works, but a *discount ratio* is genuinely low-churn even when absolute prices
move. `*float64` (not `float64`) distinguishes "unreported" from "genuinely free".

## Data Flow

    OpenRouter /api/v1/models
        │ ORModel.Pricing{Prompt, Completion, InputCacheRead}
        ▼
    enrichWithOpenRouter ──→ EnrichedModel.Pricing (*Money, now +InputCacheRead)
        │
        ▼
    collectCandidates ──(nil-price gate: Muse Spark, Ox Alpha exit here)──→ ✗
        │
        ▼
    resolveQuotaDenominators ── priceOf(m) ──→ plans.Table.Requests(ocID, price, Window5H)
        │                                            │ shape × price ÷ (tier/5) × multiplier
        │◄───────────── float64, ok ─────────────────┘
        ▼
    quota = hSat / min(reqPer5H/fw, hSat)   [unchanged]   ──→ scoredModel.reqPer5H
                                                                    │
                                                                    ▼
                                                               buildReason

## File Changes

| File | Action | Description |
|---|---|---|
| `internal/profile/plans/opencode-zen-go.yaml` | Modify | Full rewrite: 22 models × {shape, cache_read_ratio, tier_usd, multiplier}; `measured_at` → `curated_at` |
| `internal/profile/plans/plans.go` | Modify | New `Shape`/`ModelPlan`/`Price`/`Window` types; `CostPerRequest`; price-aware `Requests` |
| `internal/profile/plans/plans_test.go` | Create | Table-driven parse + derivation + reference-reproduction |
| `internal/api/models.go` | Modify | `Money.InputCacheRead *float64` |
| `internal/api/match.go` | Modify | Copy `InputCacheRead` in `enrichWithOpenRouter` |
| `internal/api/recommend.go` | Modify | `priceOf` helper; `scoredModel.reqPer5H`; `resolveQuotaDenominators`; `buildReason` text |
| `internal/api/recommend_test.go` | Modify | Denominator/fallback/reason cases |
| `openspec/specs/plan-quota-currency/spec.md` | Modify | Embedded Quota Table → shape/tier table; Staleness Surfacing → curation date |

## Interfaces / Contracts

```go
// internal/profile/plans — no api import; Price is the seam.

type Shape struct{ Input, Cached, Output float64 }

type ModelPlan struct {
    Shape          Shape   `yaml:"shape"`
    CacheReadRatio float64 `yaml:"cache_read_ratio"`
    TierUSD        float64 `yaml:"tier_usd"`
    Multiplier     float64 `yaml:"multiplier"` // 0 => 1
}

type Table struct {
    Plan      string               `yaml:"plan"`
    CuratedAt string               `yaml:"curated_at"` // was MeasuredAt
    Models    map[string]ModelPlan `yaml:"models"`
}

// Price is live per-1M-token pricing. CacheReadPerM nil => unreported by the
// price source; falls back to InputPerM * CacheReadRatio (design D5).
type Price struct {
    InputPerM, OutputPerM float64
    CacheReadPerM         *float64
}

type Window int
const (Window5H Window = iota; WindowWeek; WindowMonth)

// windowDivisor: budget_usd = tier_usd / divisor. Design D1.
var windowDivisor = map[Window]float64{Window5H: 5, WindowWeek: 2, WindowMonth: 1}

// ok is false when ocID is untabulated or the derived cost is not positive.
func (t *Table) CostPerRequest(ocID string, p Price) (float64, bool)
func (t *Table) Requests(ocID string, p Price, w Window) (float64, bool)
```

YAML shape (per model):

```yaml
grok-4.5:
  shape: {input: 1100, cached: 71500, output: 220}
  cache_read_ratio: 0.15
  tier_usd: 15
  multiplier: 1
```

`Requests` returns `float64`, not `int`: it feeds float arithmetic in `headroom`, and
rounding discards precision for no gain. `scoredModel.reqPer5H` caches the value so
`buildReason` never re-derives — this guarantees the displayed number equals the
ranked one, and avoids re-projecting price at a second call site.

`buildReason` (quota arm, Spanish preserved — existing user-facing surface):

    "Mejor calidad/cuota: %s, ~%.0f req/5h (derivado del precio actual; supuestos
     curados %s), $%.3f/M input%s%s — %s"

Fallback arm: `"(sin cuota medida)"` → `"(sin datos de cuota)"`. "medido" is removed
everywhere.

## Testing Strategy

Zero tests exist on the three touched functions. Table-driven throughout (`t.Run`),
no `t.TempDir()` or network needed — the YAML is embedded.

| Layer | What | Approach |
|---|---|---|
| Unit — `plans` | `parseTable` valid / bad `curated_at` / malformed YAML | Table-driven, explicit error cases |
| Unit — `plans` | `CostPerRequest` Grok = $0.02497, Hy3, DeepSeek off-peak; untabulated → `ok=false`; zero price → `ok=false` | Fixed-input golden values |
| Unit — `plans` | D5 fallback: live cache-read used when set; `ratio × input` when nil; explicit 0 honored | Three-case table |
| Unit — `plans` | All 3 windows; Hy3 `multiplier: 8` | Ratio assertions (5h×5 == month) |
| Fixture — `plans` | Derived 5h vs `reference-data.md` published, all 22 models | Two assertions per row: exact derived golden, **and** within 40% of published (shape rounding drifts 10–36%) |
| Unit — `api` | `resolveQuotaDenominators`: saturating → `quota==1`; scarce → `>1`; untabulated → `price/P_min`, `hasQuota=false`; nil table → all fallback; empty slice → no panic | Table-driven |
| Unit — `api` | **D3 lock**: curated + nil price → absent from `collectCandidates`, no panic, not on the bridge | Explicit regression case |
| Unit — `api` | `enrichWithOpenRouter`: `InputCacheRead` present → copied; absent → nil | Extend `match_test.go` |
| Unit — `api` | `buildReason`: quota arm contains "derivado" + `CuratedAt`, never "medido"; fallback arm | String-contains table |

## Threat Matrix

N/A — no routing, shell, subprocess, VCS/PR automation, executable-file classification,
or process-integration boundary. Pure arithmetic over already-fetched data; no new I/O.

## Migration / Rollout

No migration. No persisted state, on-disk user config, or wire format changes for input.
**One output-surface caveat**: `Money` gains a serialized field, so `EnrichedModel`
JSON goldens covering priced models need regeneration (`-update`, inspect diff, rerun
without). `omitempty` on `*float64` keeps rows byte-identical where OpenRouter reports
no cache-read price. Rollback is the proposal's single-commit revert.

## Sequencing for sdd-tasks

The schema change breaks `Requests`' two call sites, so it cannot be split from them.
Suggested slices, each independently green:

1. `Money.InputCacheRead` + `match.go` + test — no ranking behavior change (~60 lines).
2. Characterization tests pinning *today's* saturation / fallback / reason contract
   against the static table (~120 lines) — RED net before the rewrite.
3. `plans` schema + YAML + derivation + `plans_test.go` + both `recommend.go` call
   sites (~330 lines) — irreducibly atomic.
4. `buildReason` text + `spec.md` rewording + reason tests (~80 lines).

`400-line budget risk: Medium-High` — slice 3 alone approaches the budget; chained PRs
recommended. Slice 2 must land before slice 3.

## Open Questions

- [ ] None blocking. D1, D3, D4, D5 are resolved with evidence. D2 is resolved as an
      accepted, mitigated risk with the un-run spike stated explicitly — it does not
      block `sdd-tasks`, since no mitigation depends on the spike's outcome.
