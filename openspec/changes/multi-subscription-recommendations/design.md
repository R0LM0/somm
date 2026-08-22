# Design: Multi-Subscription Recommendations

## Technical Approach

The local `opencode` CLI becomes an optional enrichment source behind a `ProviderDiscoverer`
interface, merged into `ListModels` alongside today's OC Go/Zen fetch and ahead of OpenRouter
enrichment. Ranking math is unchanged except one new hard pre-filter. Every failure mode is
off-path: `ListModels` warns and returns exactly today's result.

## Architecture Decisions

| # | Decision | Choice | Rejected | Rationale |
|---|---|---|---|---|
| D1 | Invocation | One `opencode models --verbose`, no provider arg, no `--refresh` | `auth list` + N per-provider calls | `providerID` is on every record, so one process replaces N+1; `auth list` is human-formatted text, a far more fragile coupling than JSON. `--refresh` turns a local read into N network calls inside an MCP request. Escape hatch: `SOMM_OC_DISCOVERY_REFRESH=1`. |
| D2 | Placement | `internal/api/discover.go`, package `api` | New `internal/api/ocli` sub-package | The repo is flat files in one package (`api.go`, `match.go`, `provider.go`…); the interface, not a package boundary, is the test seam. |
| D3 | Latency | 5s `exec.CommandContext` timeout; in-process TTL cache (success 5m, failure 60s) + single-flight | No cache; local file cache | The MCP server is long-lived over stdio, so a process cache captures nearly all benefit; TTL bounds staleness when a provider is authenticated mid-session; a file cache adds cross-process invalidation and a `dynamic-paths` surface for nothing. Negative caching stops a missing binary from spawning-and-failing per tool call. |
| D4 | Failure taxonomy | Binary absent / non-zero exit / malformed JSON / timeout all collapse to one `slog.Warn` + continue | Distinct handling per class | Uniformly implementable: all return `(nil, error)`; `ListModels` mirrors the existing `fetchOpenRouter` warn arm. **One exception needs distinct treatment:** a *per-entry* malformed record (missing `id`/`providerID`, null `cost`) MUST drop that entry only, not the whole result — schema drift on one provider must not erase the others. |
| D5 | Identity + dedupe | Every model carries a non-empty `ProviderID`. HTTP-sourced → `"opencode"`; CLI providers `opencode`/`opencode-go`/`opencode-zen` normalize to `"opencode"`. `mergeKey = ProviderID + "/" + slug`, and `OCID` stays the bare slug only when `ProviderID == "opencode"` | Match by `ocId` alone; match by `PROVIDER_ALIASES` family | The CLI's `(providerID, id)` namespace is **not** OC's `OCModel.ID` namespace (OC IDs are bare slugs — `grok-4.5`, `gpt-5.6-luna` — per the embedded plan table). Bare-slug matching would collide `openai/gpt-5.6` with OC Go's `gpt-5.6`, which are genuinely different purchases at different prices. Family matching would wrongly fuse them. |
| D6 | Price precedence (product #2) | CLI cost overwrites the merged model's `Pricing`, sets `PriceSource: "opencode-cli"`; `enrichWithOpenRouter` MUST skip the pricing overwrite when that flag is set while still copying benchmarks/context/reasoning | Merge after enrichment | Enrichment currently overwrites `Pricing` unconditionally, so without this guard the OpenRouter proxy price silently wins — a wrong number, not merely incomplete. |
| D7 | Cost units | `cost.*` is USD **per 1M tokens** (models.dev convention); `Money` is per **token**. Convert `/1e6` in one pure mapper; `DiscoveredModel` fields named `InputPerM`/`OutputPerM`/`CacheReadPerM` to match `plans.Price` | Assume per-token | A 10⁶ unit error is the highest-consequence silent bug here. Naming makes the unit visible at the type level; conversion is testable in isolation. |
| D8 | Scoping schema (product #3) | `Selection.Providers []string` (`yaml:"providers,omitempty"`); nil inherits, non-nil replaces, explicit `[]` is a load error | Field on `Role`; union/append merge | `Selection` gets the role→profile→`{value,usd}` merge for free via `resolveSelection`, and scalar-replace matches `Objective`/`Currency` and `mergeDefaults`' `Requires`. Fail-loud on `[]` follows the repo's stated convention over an un-expressible footgun. |
| D9 | Identifier validation | Free-form: non-empty, trimmed, matched case-insensitively against `ProviderID`. Never validated against live host state | Validate against `opencode auth list` | Host-state validation makes a profile that loads on one machine fail on another and fail when `opencode` is absent — directly contradicting product #1 and the byte-identical success criterion. An unconfigured provider simply yields no candidates. |
| D10 | Pre-filter site | First check in `collectCandidates` step (3), the hard-constraint tier | Filter in `RecommendConfig`; filter in `findBestModel` | Same tier as `MinContext`/`MaxInputPrice`/`Requires` means it is never relaxed and applies automatically on both the max-2 and max-3 passes. Cheapest check first. |
| D11 | No satisfying provider | Reuse the existing nil-`findBestModel` path: a `Recommendation` with empty `Model` and a scope-specific `Reason` | New error shape; fail the tool | Per-role graceful degradation already exists and `FormatRecommendations` already renders the empty-`Model` arm. Other roles stay intact. |
| D12 | $0 exclusion visibility (product #4) | Extend `ProviderStatus` with `Ranked bool` + `ExcludedReason string`, one entry per discovered provider | Per-recommendation `ExcludedModels`; new top-level `Notes` | Excluded candidates are **not surfaced anywhere today** — only winners get a `Reason`, so some surfacing must be added. The limitation is provider-level, not per-model or per-role, and `ProviderStatus` is the existing "what sources are in play" channel `RecommendConfig` already returns and `FormatRecommendations` already renders. Stated once, not N times. |

`buildReason`'s `subInfo` currently falls through to `" (Zen)"` for anything not `go`/`both` — a
non-OC provider would be mislabelled. It must emit the provider name instead.

## Data Flow

    ListModels ──┬─→ fetchOC(go)    ─┐
                 ├─→ fetchOC(zen)   ─┤ (same WaitGroup: max, not sum)
                 └─→ Discoverer     ─┘
                          │ err → slog.Warn, continue
                          ▼
                   mergeDiscovered ──→ enrichWithOpenRouter
                   (D5 key, D6 flag)    (skips price if PriceSource set)
                          ▼
              collectCandidates ─→ providers pre-filter (D10) ─→ ranking

## File Changes

| File | Action | Description | Est. lines |
|---|---|---|---|
| `internal/api/discover.go` | Create | Interface, DTOs, `execDiscoverer`, cache, parser | 200–240 |
| `internal/api/discover_test.go` | Create | Parser table tests, cache, fake runner, integration | 260–320 |
| `internal/api/models.go` | Modify | `ProviderID`/`ProviderName`/`ModelSlug`/`PriceSource` (all `omitempty`); `Client.Discoverer` | 30–40 |
| `internal/api/api.go` | Modify | Concurrent discovery, `mergeDiscovered`, `mergeKey`, provider attribution | 90–120 |
| `internal/api/match.go` | Modify | Price-source guard; `MatchOR` on slug not namespaced `OCID` | 15–20 |
| `internal/api/recommend.go` | Modify | Pre-filter, `ProviderStatus` fields, scope reason, `buildReason` fix, `slugOf`/`familyOf`, formatter | 110–150 |
| `internal/profile/profile.go` | Modify | `Selection.Providers` | 10–14 |
| `internal/profile/load.go` | Modify | Validation + merge precedence | 30–40 |
| `internal/api/api_test.go` | Modify | Merge + degradation via fake discoverer | 180–220 |
| `internal/api/recommend_test.go` | Modify | Scoping, excluded status, reason text | 200–260 |
| `internal/api/match_test.go` | Modify | Price-source guard | 50–70 |
| `internal/profile/load_test.go` | Modify | Providers validation/merge | 90–120 |
| `cmd/somm/main.go` | Modify | `SOMM_DISCOVERY` help text | 8–12 |

**Total ≈ 1,270–1,630 authored lines.**

## Interfaces / Contracts

```go
// DiscoveredModel is one model reported by the local opencode CLI. Prices are
// USD per 1M tokens (D7), matching plans.Price, not api.Money.
type DiscoveredModel struct {
	ProviderID    string
	ID            string
	Name          string
	InputPerM     float64
	OutputPerM    float64
	CacheReadPerM *float64
	ContextLength *int64
}

// ProviderDiscoverer reports the models served by providers configured on this
// host. Every error is non-fatal: callers warn and continue (D4).
type ProviderDiscoverer interface {
	Discover(ctx context.Context) ([]DiscoveredModel, error)
}

// Client.Discoverer: nil means the default execDiscoverer; tests inject a fake
// or a no-op. SOMM_DISCOVERY=off disables the default (rollback path).
```

```yaml
selection:
  objective: value
  currency: usd
  providers: [opencode, kimi-for-coding]   # omit = all (product #3)
```

## Testing Strategy

| Layer | What | Approach |
|---|---|---|
| Unit (pure) | `parseDiscoverOutput`, unit conversion, `mergeKey`, provider normalization | Table-driven over CLI-output fixtures: valid, empty array, malformed JSON, missing `id`/`providerID`, null `cost`, unknown extra fields, oversize |
| Unit (seam) | Merge, D6 precedence, degradation, pre-filter, scope reason, `ProviderStatus` | `fakeDiscoverer` returning canned models or each error class; existing `httptest` for OC/OR |
| Unit (schema) | `Selection.Providers` validation + role→profile precedence | Table-driven in `internal/profile` |
| Unit (subprocess) | Timeout, non-zero exit, single-flight, TTL | Injected `runner func(ctx) ([]byte, error)` behind `execDiscoverer` |
| Integration | Real `opencode models --verbose` | `if testing.Short() { t.Skip }` **and** skip when `exec.LookPath("opencode")` fails; asserts a sane $/M range to catch a D7 unit error |

## Threat Matrix

Subprocess/process-integration boundary — **Applicable**. The reference matrix's five VCS rows
are all `N/A` (no Git, commit, push, PR, or file-classification surface in this change).

| Case | Safe behavior | RED test |
|---|---|---|
| Binary resolution | `exec.LookPath("opencode")` only. Never a shell, never `sh -c`, never a user-supplied path | Assert argv is exactly `{"models","--verbose"}`; no user input is ever interpolated |
| PATH hijack | Output is decoded into a typed struct, never executed; only numeric prices and opaque ids are consumed — no path or command fields | Hostile fixture with unknown fields and oversized strings is ignored |
| Unbounded stdout | `io.LimitReader` at 8 MiB; over-limit is treated as malformed → warn + continue | Oversize fixture |
| Hang / no exit | `exec.CommandContext` with the 5s timeout; process killed and reaped | Runner blocking past the deadline |
| Non-zero exit | stderr captured, truncated to 512 bytes before logging | Exit-1 fixture |
| Concurrent tool calls | Single-flight: at most one process | Parallel `Discover` with a counting runner |

## Migration / Rollout

No migration. Additive and off-path on failure. `SOMM_DISCOVERY=off` disables discovery without
a revert; absent `opencode`, all new fields are `omitempty` so existing output is byte-identical.

## Sequencing (for sdd-tasks)

**Decision needed before apply: Yes. Chained PRs recommended: Yes. 400-line budget risk: High.**

| Slice | Contents | Est. | Depends on |
|---|---|---|---|
| A | `discover.go` types + interface + parser; parser tests | ~380 | — |
| B | `execDiscoverer`: subprocess, timeout, cache, single-flight + tests | ~350 | A |
| C | `models.go` fields + `api.go` merge + `match.go` guard + tests | ~400–480 | A |
| D | `profile.go` + `load.go` + `load_test.go` | ~150–200 | — (can run first/parallel) |
| E | `recommend.go` pre-filter, `ProviderStatus`, reasons, formatter + tests | ~350–420 | C, D |

**Irreducible atomicity:** slice C must contain both the `api.go` merge and the `match.go`
enrichment guard (D6). Shipping the merge alone produces a *wrong price*, not merely an
incomplete feature. Slice E's pre-filter and its no-candidate reason are likewise atomic — a
filter without the reason yields a silent empty result.

## Open Questions

- [x] **RESOLVED by orchestrator, verified with live shell access before `sdd-tasks`:**
  - `cost.*` unit: `opencode models opencode-go --verbose` for `grok-4.5` returns `"cost": {"input": 2, ...}`,
    matching the known real price of $2.00/M input tokens exactly. Confirmed **USD per 1M tokens**
    (matches `api.Money`'s `*PerM` convention) — not per-token (would have read `0.000002`).
  - `opencode models --verbose` with no provider argument: confirmed it emits all 4 of this user's
    configured providers in one call (`kimi-for-coding`, `openai`, `opencode`, `opencode-go`),
    matching D1's single-invocation design.
  Both of slice A's assumptions are now confirmed, not just assumed; the sane-range assertion
  remains as a regression guard, not the only line of defense.
