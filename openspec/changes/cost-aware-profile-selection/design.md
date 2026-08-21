# Design: Cost-Aware Profile Selection

## Technical Approach

Two independent axes are added to the profile schema — `selection.objective` (which comparator
ranks candidates) and `selection.currency` (which denominator the comparator divides by) — plus a
per-role `frequency` read only under `currency: quota`. Both resolve **at load time** into every
`Role` through the same `mergeDefaults` pass that already inherits `min_context`/`max_input_price`,
so `findBestModel` reads `role.Selection` directly and needs no profile-level plumbing. A new
`internal/profile/plans` package embeds the quota table, mirroring `preset.go`'s `//go:embed`. The
tier is captured once by a new `somm setup` screen and persisted as `SOMM_OC_TIER` through the
existing `saveEnvFile` allowlist. The hard-constraint pre-filter, max-2/relax-3 spreading, and the
`Qraw`-never-`Qnorm` invariant (sibling Decisions 1, 2, 5) are untouched.

## Architecture Decisions

### Decision 1 (LOAD-BEARING): the `value`/`usd` arm is the *unedited* existing code, not equivalent code

Parity is asserted structurally, not argued. `findBestModel`'s existing weighted `sort.Slice`
closure and `buildReason`'s existing `fmt.Sprintf` line are moved verbatim into a `case "value":`
arm whose guard cannot be reached by a profile that omits `selection`. Every new field is either
unread on that path or resolves to the value the code already assumed.

| Addition | Value when absent | Read on `value`/`usd`? | Why byte-identical |
|---|---|---|---|
| `Profile.Selection` | `{value, usd}` | objective only | selects the unedited closure |
| `Role.Selection` | inherits profile → `{value, usd}` | objective only | merge *fills a new field*; mutates no existing field |
| `Role.Frequency` | `""` | **no** | only `frequencyWeight` reads it, only under `quota` |
| `plans` table | not loaded | **no** | `RecommendConfig` skips the load unless some role resolves to `quota` — zero I/O |
| `scoredModel.quota`, `.hasQuota` | `0`, `false` | **no** | not referenced by the value comparator |
| `SOMM_OC_TIER` | unset | → `usd` | absent key is today's state |
| `presets/gentle-ai.yaml` | no `selection:`, no `frequency:` | — | **file has 0 diff lines in both PRs** |

**Mechanical proof**: `internal/api/testdata/gentle-ai.golden` must have **0 diff lines** across both
PRs, and `TestGoldenParity` must pass unmodified. If either PR touches the golden, parity broke.

### Decision 2 (LOAD-BEARING): the quota denominator needs a saturation cap, or `frequency` cannot reorder anything

The spec's denominator is `D = frequency_weight / requests_per_5h`, so the ranking key is
`Qraw / D = Qraw × rp5h / fw`. **`fw` is constant across candidates within a role**, so dividing
every candidate by it is order-preserving — the uncapped form makes `frequency` a mathematical
no-op on ranking.

**Counter-example (uncapped)** — P: `Qraw=80, rp5h=110`; C: `Qraw=50, rp5h=45300`.
- `frequency: high` (fw=4): P = 2 200, C = 566 250 → C wins.
- `frequency: low` (fw=0.25): P = 35 200, C = 9 060 000 → **C still wins**. `fw` cancels; no
  inversion. This fails the proposal's success criterion *and* the spec scenario "Higher frequency
  weight favors quota-abundant models".

**Choice**: quota enters as a bounded **affordability factor** `aff = min(H, H_sat) / H_sat`, where
`H = rp5h / fw` is the role's effective headroom and `H_sat = 200` req/5h is the point past which
extra quota stops differentiating. Ranking key stays `Qraw / D` with `D = 1/aff` — algebraically the
spec's form **plus a cap**, and `/H_sat` is a per-role constant (order-neutral). The cap is exactly
what makes `frequency` load-bearing.

`frequencyWeight`: `high → 4.0`, `medium → 1.0` (also the default for `""`), `low → 0.25` — a
symmetric ×4 ladder, so one frequency step is a 4× change in bucket consumption.

### Decision 3 (LOAD-BEARING): untabulated candidates bridge into the same [0,1] scale via a set-relative price reference

The spec requires untabulated candidates to be ranked, not excluded, "using their `usd` price as the
denominator". Mixing raw units in one sort is incoherent: `Qraw × rp5h` reaches ~10⁶ while
`Qraw / price` reaches ~10², so whichever unit is larger always wins regardless of merit.

**Choice**: untabulated candidates get `aff = P_min / price`, where `P_min` is the minimum price
**over that role's constraint-filtered candidate set** — self-normalizing, no invented constant, and
precedent-consistent with `computeNormalized`, which already normalizes per role over that same set.
Both factors then mean the same thing: `aff = 1.0` ⇔ "this resource does not constrain me".

**Worked example** — P: `Qraw=80, $3.00/M, rp5h=110`; C: `Qraw=50, $0.15/M, rp5h=45300`;
U: `Qraw=65, $0.60/M, untabulated`. `P_min = 0.15`, `H_sat = 200`.

| Candidate | `aff` @ `frequency: high` (fw=4) | score | `aff` @ `frequency: low` (fw=0.25) | score |
|---|---|---|---|---|
| P | `min(27.5,200)/200 = 0.1375` | 11.00 | `min(440,200)/200 = 1.0` | **80.00** |
| C | `min(11325,200)/200 = 1.0` | **50.00** | `1.0` | 50.00 |
| U | `0.15/0.60 = 0.25` | 16.25 | `0.25` | 16.25 |

`high` → C wins (cheap high-quota beats premium low-quota); `low` → P wins. **Inversion achieved**,
and U is ranked, never excluded, never an error.

**Rejected**: two-tier ranking (tabulated group always above untabulated) — coherent but reads
"MUST still appear in the ranked results" too weakly; a fixed global reference price — an invented
magic constant with no set-relative meaning.

### Decision 4: comparators, with a *total* order for `quality`

| Objective | Sort keys | Note |
|---|---|---|
| `value` (default) | `Qraw/D` desc → `Qraw` desc | unedited existing closure |
| `quality` | `Qraw` desc → `D` asc → `OCID` asc | price consulted **only** on ties |
| `budget` | `value` keys, over the `max_input_price`-filtered set | filter already exists in `collectCandidates` step 3 |

`quality` **requires** the `OCID` third key: single-metric roles make `Qraw` ties common, and
`sort.Slice` is unstable — without it, the winner is nondeterministic run to run and the golden test
becomes flaky. `value` keeps only two keys, verbatim (Decision 1).

`budget` adds no new filter code: `max_input_price` is already a hard pre-filter. With no effective
cap set, `budget` degrades to `value` — see Open Questions.

### Decision 5: selection resolves at load; the tier refines it afterward, never leaving half-state

`Load` fills `Role.Selection` field-by-field (role → profile → `{value, usd}`) and records an
unexported `currencyExplicit bool`. `Load` therefore always yields a **complete, usable** profile.
`ResolveWithTier(flagPath, tier)` then overwrites currency **only** where `!currencyExplicit`, so an
explicit `selection.currency` always beats the tier. `Resolve(flagPath)` becomes
`ResolveWithTier(flagPath, "")` — its 4 existing callers and signature are untouched.

`TierCurrency`: `"" → usd`; `go|zen → quota`; anything else → **error** (fatal in `run()`), matching
the sibling's Decision 6 fail-loud stance. `go` vs `zen` is stored rather than a boolean so a future
per-tier table split needs no re-prompt.

**Validation** splits in two: existing pre-merge `validate()` gains the `objective`/`currency`/
`frequency` enum checks (unknown value = error, never silent fallback); a new post-merge pass runs
after `mergeDefaults` for anything depending on merged state.

### Decision 6: `Reason` strings — one arm is the untouched original

| Case | Format |
|---|---|
| `value` + `usd` | `Mejor relación calidad/precio: %s, $%.3f/M input%s%s — %s` **(unchanged)** |
| `quality` + `usd` | `Máxima calidad disponible: …` |
| `budget` + `usd` | `Mejor calidad/precio bajo $%.2f/M: …` |
| `quota`, tabulated | `Mejor calidad/cuota: %s, %d req/5h (medido %s), $%.3f/M input…` |
| `quota`, fallback | `Mejor calidad/precio (sin cuota medida): …` — **no date**, per spec |

## Data Flow

    SOMM_OC_TIER (.env) ─┐
    --profile/SOMM_PROFILE ├─► profile.ResolveWithTier ─► *Profile (every Role.Selection complete)
                          ┘                                    │
    plans.OpenCodeZenGo() ──► *plans.Table ──► RecommendConfig ─┘   (loaded only if any role → quota)
                                                    │
                                findBestModel ─► collectCandidates (unchanged pre-filter)
                                                    │  fills qraw, price, quota, hasQuota
                                                    ▼
                                   objective switch × currency denominator ─► buildReason

## File Changes

| File | Action | PR | Description |
|---|---|---|---|
| `internal/profile/profile.go` | Modify | 1 / 2 | `Selection` type + `Profile.Selection` + `Role.Selection` (1); `Role.Frequency`, `FrequencyWeight` (2) |
| `internal/profile/load.go` | Modify | 1 / 2 | enum validation, selection merge, post-merge pass |
| `internal/profile/resolve.go` | Modify | 2 | `ResolveWithTier`, `TierCurrency`, `ApplyTierCurrency` |
| `internal/profile/plans/plans.go` | Create | 2 | `//go:embed` loader, `Table`, `Requests(ocID) (int, bool)` |
| `internal/profile/plans/opencode-zen-go.yaml` | Create | 2 | `plan`, `measured_at`, `models: ocId → requests_per_5h` |
| `internal/api/recommend.go` | Modify | 1 / 2 | objective switch + `buildReason` (1); `aff`/saturation, `scoredModel.quota` (2) |
| `cmd/somm/main.go` | Modify | 2 | read `SOMM_OC_TIER`, `ResolveWithTier`, fatal on bad tier |
| `cmd/somm/tui.go` | Modify | 2 | `scrTier` screen, `tiers`/`tierCursor`, `updateTier`, view |
| `cmd/somm/setup.go` | Modify | 2 | append `SOMM_OC_TIER` to `saveEnvFile`'s `order` |
| `internal/api/testdata/gentle-ai.golden` | **Unchanged** | — | 0 diff lines = the parity proof |
| `internal/profile/presets/gentle-ai.yaml` | **Unchanged** | — | parity via defaults |

## Interfaces / Contracts

```go
type Selection struct {
    Objective string `yaml:"objective,omitempty"` // value | quality | budget
    Currency  string `yaml:"currency,omitempty"`  // usd | quota
}
type Role struct {
    /* existing fields unchanged */
    Selection        *Selection `yaml:"selection,omitempty"` // non-nil & complete after Load
    Frequency        string     `yaml:"frequency,omitempty"` // high | medium | low
    currencyExplicit bool       // unexported: set during merge, not YAML
}
func FrequencyWeight(freq string) float64                       // high 4.0 | medium/"" 1.0 | low 0.25
func TierCurrency(tier string) (string, error)                  // "" → usd; go|zen → quota; else error
func ResolveWithTier(flagPath, tier string) (*Profile, error)   // Resolve(p) == ResolveWithTier(p, "")

// internal/profile/plans
type Table struct {
    Plan       string         `yaml:"plan"`
    MeasuredAt string         `yaml:"measured_at"` // validated as 2006-01-02 at load
    Models     map[string]int `yaml:"models"`
}
func OpenCodeZenGo() (*Table, error)
func (t *Table) Requests(ocID string) (int, bool)               // ok=false for absent — never 0-as-default
```

Nothing marshals `Profile`/`Role` back to YAML (verified: no `yaml.Marshal` in the repo), so new
fields cannot leak into any output. `RecommendConfig`'s exported signature is unchanged.

**Wizard flow**: `scrTier` is appended **last** in the `screen` iota block so no existing constant
renumbers. Entry point is `updateProviderKeyInput` where `m.keyIdx == len(m.keyFields)-1` — today it
jumps to `scrConfigProgress`; it now enters `scrTier`, which on `enter` sets
`m.keys["SOMM_OC_TIER"]` and then jumps to `scrConfigProgress`. `esc` returns to the last key field
(no skip — D1 fixes the domain to `go|zen`). On entry the cursor preselects any tier already in
`.env`. **Data-loss check**: `saveEnvFile` rewrites `.env` wholesale from its `order` allowlist, so
the tier must be re-collected on every configure run or it would be dropped — it is, because
`scrTier` is unconditional in that path, and preselection makes re-answering one keypress.

## Testing Strategy

| Layer | What to test | Approach |
|---|---|---|
| **Golden (KEY)** | default output unchanged | `TestGoldenParity` unmodified vs. `testdata/gentle-ai.golden`, byte-for-byte, in **both** PRs |
| Unit — loader | enum rejection (`objective`/`currency`/`frequency`), field-by-field override, `currencyExplicit` | table-driven, `t.Run` per scenario |
| Unit — comparators | Decision 4 rows; `quality` Qraw-tie → lower price → OCID; `budget` excludes over-cap | table-driven, fixed candidate sets |
| Unit — quota | Decision 3's exact numeric table (high **and** low, asserting inversion); `P_min` bridge; `H_sat` boundary at exactly 200 | table-driven |
| Unit — fallback | untabulated candidate ranked, not excluded, no error; all-untabulated role | table-driven |
| Unit — reason | `value`/`usd` string byte-identical; quota carries `measured_at`; fallback carries none | table-driven |
| Unit — plans | known id, absent id → `ok=false`, `measured_at` parses as `2006-01-02` | table-driven; embedded, no I/O |
| Unit — tier | `TierCurrency` domain incl. error; explicit `currency` beats tier | table-driven |
| Unit — TUI | `scrTier` transitions, preselection, `esc` back | direct `Model.Update()` with `tea.KeyMsg` (per go-testing gate) |
| Integration | `.env` round-trip retains all three keys | `t.TempDir()` + `saveEnvFile` |

## Threat Matrix

N/A — no routing, shell, subprocess, VCS/PR automation, executable-file classification, or
process-integration boundary. New surfaces are an **embedded** (non-user-supplied) YAML, an env-var
read, and a TUI selection with a closed two-value domain. The one new mutation worth an explicit
test is `saveEnvFile`'s whole-file `.env` rewrite at `0600` — the same drop-other-keys hazard that
D1 deliberately avoided in `opencode.json`; covered by the integration row above.

## Migration / Rollout

No data migration. Both axes are opt-in and additive behind defaults; PR1 is independently
revertible and leaves PR2 unbuilt.

**Correction to the proposal's Rollback Plan**: it claims a leftover `selection:` block "fails
validation as unknown keys" after a revert. Verified false — `load.go` uses `yaml.Unmarshal` without
`KnownFields(true)`, so unknown top-level and role-level keys are **silently ignored**. A reverted
install therefore keeps loading such profiles but silently reverts to `value`/`usd` ranking. That is
quieter than an error and arguably worse; the revert note must say "your `selection:` block stops
taking effect", not "your profile stops loading". A leftover `SOMM_OC_TIER` is an inert unread key.

**Review budget** (authored additions+deletions, goldens excluded):

| Slice | Scope | Est. lines | Risk |
|---|---|---|---|
| PR1 | `Selection` schema + validation + override, objective comparator, `buildReason`, tests | ~355 | Medium |
| PR2 as proposed | plans + quota ranking + frequency + tier plumbing + wizard + tests | **~600** | **High** |

**`400-line budget risk: High` for PR2 as currently scoped.** Recommended split into
**PR2a** (`plans` package, embedded table, `Role.Frequency`, quota comparator, `Reason`, tests —
~380) and **PR2b** (`ResolveWithTier`, `main.go` wiring, `scrTier` wizard, `saveEnvFile`, tests —
~220). This preserves D2's stacked-to-main topology and only changes the slice count; the orchestrator
owns that call, not this design.

## Open Questions

- [ ] **`setup-wizard` spec gap (blocks PR2b only)**. `sdd-spec` wrote no `setup-wizard` delta, but
      the tier step *is* a contract change, not an implementation detail: the archived
      `Requirement: Persistence` enumerates exactly which keys `.env` MUST contain, and this adds a
      third; a new blocking screen is inserted between `Key Prompts` and `Persistence`; and D1's
      Success Criterion ("asks once, persists, does not re-ask") is unverifiable without it. **A
      follow-up `sdd-spec` pass MUST add a `setup-wizard` delta before `sdd-tasks`/`sdd-apply` reach
      PR2b** — shape: MODIFIED `Persistence` (adds `SOMM_OC_TIER`), ADDED `Tier Capture` (asked once
      after key prompts, domain `go|zen`, preselected from `.env`, explicit `selection.currency`
      overrides). PR1 and PR2a are fully spec-backed and unaffected.
- [ ] `budget` with no effective `max_input_price` currently degrades to `value` silently. The
      fail-loud house style argues for a validation error, but the written `weighted-scoring` delta
      does not require one, so this design conforms rather than inventing a requirement. Recommend
      tightening the spec.
- [ ] `quality` on a weightless role ranks by the `50.0` intelligence fallback, which can float a
      benchmark-less model above a real 45.0 one. Spec-permitted; document as a gotcha or exclude
      null-intelligence candidates under `quality`.
- [ ] `H_sat = 200` req/5h and the ×4 frequency ladder are calibrated judgement, not measurement.
      Both live as single named constants so recalibration is a one-line change.
