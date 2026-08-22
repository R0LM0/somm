# Proposal: Multi-Subscription Recommendations

## Intent

SOMM ranks only OpenCode Go/Zen models from two hardcoded HTTP endpoints, priced indirectly via
OpenRouter alias-matching. Users who pay for several providers already configured in OpenCode
(OpenAI, Kimi For Coding, OpenCode Go, GitHub Copilot) get recommendations that ignore most of
what they own and can access. SOMM should recommend across the subscriptions the user actually
has, at first-party live prices, without asking them to declare that inventory by hand.

## Scope

### In Scope
- Optional discovery source: local `opencode` CLI (`auth list`, `models [provider] --verbose
  --refresh`) for configured providers and live per-model `cost.*`.
- Graceful degradation: binary absent, unauthenticated, malformed, or slow output MUST warn and
  continue with today's OC Go/Zen + OpenRouter behavior. Never fail `list_available_models` or
  `recommend_config`.
- Provider identity on catalog models; merge with the existing OC catalog.
- Provider-scoping control on `Role`/`Selection` to restrict a role to chosen providers.
- Mockable interface at the subprocess boundary; `testing.Short()`-skippable integration tests.

### Out of Scope
- Quota-aware ranking for $0-price OAuth-gated providers (OpenAI oauth, GitHub Copilot) — no
  public usage-cap source exists. Preserve today's `collectCandidates` nil/zero-price exclusion.
- Inspecting or reusing `gentle-ai` code (separate codebase; UX precedent only).
- Per-provider first-party HTTP clients or per-provider API-key prompts.
- Replacing the OC Go/Zen endpoints or the OpenRouter enrichment path.
- Reading OpenCode's `auth.json` or other private local files.

## Capabilities

### New Capabilities
- `multi-provider-catalog`: optional local-CLI discovery of configured providers and live-priced
  models, its degradation contract, and its merge into the catalog.

### Modified Capabilities
- `role-profiles`: `selection` gains a provider-scope field under the existing
  role→profile→default merge and fail-loud validation.
- `weighted-scoring`: provider scope acts as a hard pre-filter; defines behavior when no
  configured provider satisfies a role.

## Confirmed Product Decisions

These were open questions when this proposal was first drafted (auto mode, no interactive
pause); the user confirmed all four explicitly before `sdd-spec`/`sdd-design` started:

1. **Default posture**: discovery is **on by default**, degrading silently (warn + continue) if
   `opencode` is absent/unauthenticated/malformed — matches the existing OpenRouter precedent.
2. **Duplicate-model price precedence**: when the same model is reachable via both the `opencode`
   CLI and the OC Go/Zen endpoints, the **CLI's first-party price wins** — it resolves the
   proxy-price-accuracy risk `live-quota-derivation` flagged for OpenRouter, rather than deferring
   to the less-trustworthy source.
3. **Scoping default**: a role ranks against **all configured providers by default**;
   provider-scoping on `Role`/`Selection` is an opt-in narrowing, not an opt-in inclusion.
4. **Excluded-$0-provider visibility**: when a $0-price OAuth-gated model (OpenAI oauth, GitHub
   Copilot) is excluded from ranking, the system MUST surface an explicit reason ("flat-rate
   subscription, no usage-cap ranking available") rather than silently omitting it — so this
   reads as a known limitation, not a bug.

A fifth question (whether `somm setup` should show a detected-providers confirmation screen) was
left as proposed — non-load-bearing, optional UX — not re-asked.

## Approach

Exploration Approach 1, narrowly scoped: the CLI becomes an optional enrichment source behind a
small discovery interface, mirroring today's OpenRouter warn-and-continue handling. Ranking math
is unchanged apart from the new provider pre-filter.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/api/` (new discovery unit) | New | CLI invocation, parsing, degradation |
| `internal/api/api.go`, `models.go` | Modified | Merge source; provider identity field |
| `internal/profile/{profile,load,resolve}.go` | Modified | Provider scope + merge/validation |
| `internal/profile/rank*.go` | Modified | Provider pre-filter |
| `cmd/somm/setup.go` | Modified | Optional provider detection/confirmation |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| `opencode` binary absent on SOMM host | High | Optional source; warn + continue |
| CLI flags/output are not a versioned API | Med | Parse defensively; degrade on schema drift |
| Catalog counts drift between runs | Med | Surface staleness; do not assume determinism |
| Subprocess latency in MCP server | Med | Timeout + caching decided in design |
| Architecture shift (HTTP-only → subprocess) | Med | Explicit, named, optional-only decision |

## Rollback Plan

Discovery is additive and off-path on failure. Revert by disabling the discovery source (env
flag/config) or reverting the change commits; the OC Go/Zen + OpenRouter path is untouched.

## Dependencies

- Local `opencode` CLI, installed and authenticated — optional, never required.

## Success Criteria

- [ ] Models from every configured provider appear in recommendations.
- [ ] Prices for discovered providers come from first-party `cost.*`.
- [ ] All existing behavior is byte-identical when `opencode` is absent or unauthenticated.
- [ ] A role can be scoped to a provider subset and ranks only within it.
- [ ] `go test -cover ./...` passes with discovery mocked; integration skipped under `-short`.
