# Somm

A model advisor for your AI agents. Fetches the models you actually have access
to from OpenCode subscriptions and OpenRouter, cross-references benchmarks and
pricing, and recommends the best model for each agent role — with the reasoning.

It's an [MCP](https://modelcontextprotocol.io) server, so it plugs into whatever
agent host you use.

[![Tests](https://github.com/R0LM0/somm/actions/workflows/test.yml/badge.svg)](https://github.com/R0LM0/somm/actions/workflows/test.yml)
[![Release](https://github.com/R0LM0/somm/actions/workflows/release.yml/badge.svg)](https://github.com/R0LM0/somm/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

## Features

- **Interactive setup wizard** — a console TUI that detects missing API keys and guides you through configuration
- **Console quality/price chart** — `somm chart` prints a Pareto-frontier view of every OpenRouter model (any provider, not just OpenCode Go/Zen) ranked by price, marking the ones with the best quality for their price
- List available models from OpenCode Go/Zen subscriptions
- **Automatic provider discovery** — on, by default, reads every provider already configured in your local `opencode` CLI (Anthropic, OpenAI, Kimi, or anything else you've logged into) and folds their models straight into the catalog, no extra API key needed
- **Provider scoping per role** — restrict a role's ranking to specific providers with `selection.providers`
- Cross-reference with OpenRouter benchmarks and pricing
- Read agent selection criteria from the Gentle AI guide
- Search specific model benchmarks
- **Recommend optimal model configuration per agent role**
- **Estimate monthly costs by agent**
- **Compare models side-by-side**
- **Validate current configuration**
- **Export safe config to opencode.json** (only the `model` field)
- Automatic `.env` configuration loading
- HTTP timeout, retry, and graceful degradation
- **Multi-provider support** — OpenCode, OpenRouter, and every provider your local `opencode` CLI knows about

## Installation

### Using Go

```bash
go install github.com/R0LM0/somm/v2/cmd/somm@latest
```

### From Release

Download the latest binary from [Releases](https://github.com/R0LM0/somm/releases).

### From Source

```bash
git clone https://github.com/R0LM0/somm.git
cd somm
make install
```

## Configuration

### Quick setup (recommended)

Just run `somm` — if no API keys are configured, the setup wizard launches automatically:

```bash
somm
```

The wizard will:
1. Check if you already have API keys configured
2. Ask whether you want to set an OpenRouter key (optional, recommended for
   benchmarks) — OpenCode Go/Zen isn't asked about at all, since
   [provider discovery](#provider-discovery) already covers it automatically
3. Guide you through pasting the key, if you opted in
4. Ask which OpenCode tier you're on (`go` or `zen`) — asked once, only on
   first setup or with `--force`
5. Save the `.env` file and update `opencode.json`
6. Start the server automatically

### Manual setup

If you prefer manual configuration, see the options below.

### Environment variables

| Variable | Required | Description |
|----------|----------|-------------|
| `OPENCODE_API_KEY` | No | OpenCode Go/Zen subscription key. Optional — [provider discovery](#provider-discovery) already covers OC Go/Zen (and any other provider) via the local `opencode` CLI when it's authenticated, no key needed. Set this only if you want the direct HTTP fetch as well/instead. |
| `OPENROUTER_API_KEY` | No | OpenRouter API key for benchmarks. Recommended: benchmarks (intelligence/coding/agentic) come from OpenRouter and nowhere else; the key itself stays optional since OpenRouter's catalog is public. |
| `SOMM_PROFILE` | No | Path to a role profile YAML file (see [Role profiles](#role-profiles)) |
| `SOMM_OC_TIER` | No | Your OpenCode subscription tier: `go` or `zen`. Captured once by the setup wizard; refines the default `selection.currency` (see [Role profiles](#role-profiles)) to `quota` for any role that doesn't set it explicitly. Unset defaults to `usd`. |
| `SOMM_DISCOVERY` | No | Set to `off` to disable local `opencode` CLI provider discovery (see [Provider discovery](#provider-discovery)). On by default; failures already degrade gracefully, this is a hard rollback switch. |
| `SOMM_OC_DISCOVERY_REFRESH` | No | Set to `1` to force a live `opencode` CLI call on every discovery instead of using the in-process cache (5 min success / 60s failure TTL). |

### .env file

Create a `.env` file next to the binary:

```
OPENCODE_API_KEY=sk-your-key-here
OPENROUTER_API_KEY=sk-or-your-key-here
SOMM_OC_TIER=go
```

### Flags

```bash
somm -opencode-api-key sk-xxx -openrouter-api-key sk-or-xxx -profile ./somm.yaml
```

### Non-interactive mode

For CI/scripts, use `--skip-setup` to fail with a clear message instead of launching the wizard:

```bash
somm --skip-setup
```

### Provider discovery

Beyond OpenCode Go/Zen and OpenRouter, Somm reads whatever providers you
already have configured in the local `opencode` CLI (`opencode auth login`) —
Anthropic, OpenAI, Kimi, or anything else — and merges their models straight
into the catalog, with real pricing when the CLI reports it. No extra API key
needed on Somm's side: it reuses whatever `opencode` itself is already
authenticated with.

This runs automatically, once per `opencode models --verbose` call (5s
timeout, cached 5 minutes on success / 60s on failure, single-flighted so
concurrent tool calls never spawn more than one process). Every failure mode
— binary absent, non-zero exit, malformed output, timeout — is non-fatal: it
warns and falls back to exactly what Somm returned before this feature
existed. Disable it entirely with `SOMM_DISCOVERY=off` if you'd rather not
shell out to `opencode` at all.

A discovered provider with no per-token pricing (a flat-rate/subscription
plan) still shows up — `recommend_config` and `list_available_models` mark it
`configured` but not `ranked`, with a reason, instead of hiding it silently.
When that reason is genuinely flat pricing (e.g. an OAuth/subscription login
like ChatGPT-style OpenAI auth) rather than missing pricing data, Somm also
looks up a *reference* list price from OpenCode's own local pricing cache
(`~/.cache/opencode/models.json`) and shows it alongside the reason —
informational only, it never affects ranking or which model wins a role,
since you aren't actually billed per token for that provider.

### Role profiles

Recommendations are driven by a `Profile`: a list of roles, each with scoring
`weights` over `intelligence`/`coding`/`agentic` benchmarks and optional hard
constraints (`min_context`, `max_input_price`, `requires`, `exclude_family_of`).
By default, Somm ships the `gentle-ai` preset (the original 19-role taxonomy)
embedded in the binary — no configuration needed.

The active profile is resolved in this order, using the first source found:

1. `-profile <path>` CLI flag
2. `SOMM_PROFILE` environment variable
3. `./somm.yaml` in the current working directory
4. `$XDG_CONFIG_HOME/somm/somm.yaml` (or `~/.config/somm/somm.yaml`)
5. the embedded `gentle-ai` preset

A malformed or invalid profile file at any of these sources fails loud — the
server does not start with a silently-substituted default. Bring your own
roles with a YAML file like:

```yaml
version: 1
defaults:
  min_context: 32000
selection:
  objective: value   # value (default) | quality | budget
  currency: usd      # usd (default) | quota
roles:
  - id: my-agent
    description: "needs strong coding + a context floor"
    criticidad: "CRÍTICO"
    weights:
      coding: 0.7
      intelligence: 0.3
    max_input_price: 5.0
    requires: ["reasoning"]
    frequency: high    # high | medium (default) | low — read only under currency: quota
    selection:
      providers: ["opencode", "kimi-for-coding"]  # omit = every discovered provider
```

#### Selection: objective and currency

Every profile and role can set a `selection` block to control ranking:

- `objective` picks the comparator: `value` (default, quality-per-price) picks
  the best ratio; `quality` maximizes raw benchmark score, breaking ties on
  price; `budget` behaves like `value` but requires an effective
  `max_input_price` ceiling (role- or `defaults`-level) — a `budget` role with
  no ceiling anywhere fails loud at load time.
- `currency` picks the denominator: `usd` (default) ranks by price; `quota`
  ranks by OpenCode plan headroom instead, using the role's `frequency`
  (`high`/`medium`/`low`, default `medium`) to weight how much quota a role is
  expected to consume. Models missing from the quota table are still ranked
  (never excluded) using a price-based fallback.

Both fields resolve independently, role overriding profile overriding the
`{value, usd}` default. Setting `SOMM_OC_TIER` (`go`/`zen`) via the setup
wizard sets the *default* `currency` to `quota` for any role that doesn't set
`selection.currency` explicitly — an explicit `selection.currency` in your
profile always wins over the tier.

#### Selection: providers

`selection.providers` restricts a role to only the listed providers (matched
case-insensitively against each model's `providerId` — see
[Provider discovery](#provider-discovery)). Omitted or `nil` ranks every
configured provider, same as before this existed; an explicit empty list
(`providers: []`) fails to load rather than silently ranking nothing. Role
overrides profile, same merge order as `objective`/`currency`. A role scoped
to a provider nobody has configured doesn't error — it just has no
candidates, and `recommend_config` says so in its reason.

## Usage

### First run

Just run `somm` — the auto-setup wizard handles everything:

```bash
somm
```

If API keys are missing, you'll see the setup wizard. If already configured, the server starts immediately.

### Setup wizard (manual)

You can also run the wizard explicitly:

```bash
somm setup
```

To reconfigure an existing installation:

```bash
somm setup --force
```

### Console quality/price chart

`somm chart` prints a ranked, ★-marked Pareto-frontier view of the full
OpenRouter catalog — no `OPENCODE_API_KEY` required, since OpenRouter's model
list is public:

```bash
somm chart                              # Pareto-optimal models by intelligence/price
somm chart --metric coding              # rank by coding score instead
somm chart --provider anthropic         # filter by provider or model name
somm chart --all --top 50               # list every priced model, not just the frontier
```

### With OpenCode (manual)

Add to your `opencode.json`:

```json
{
  "mcp": {
    "somm": {
      "type": "local",
      "command": ["path/to/somm"],
      "enabled": true
    }
  }
}
```

## MCP tools

> Example outputs below are illustrative — model names and numbers depend on the
> catalogs reachable with your keys at query time.

#### list_available_models
Fetch all available AI models from your subscriptions, plus every provider
discovered via the local `opencode` CLI (see
[Provider discovery](#provider-discovery)).

Parameters:
- `subscription`: `"go" | "zen" | "both"` (default: `"both"`)
- `enrich`: boolean (default: `true`) — cross-reference with OpenRouter

Discovered models carry `providerId`, `providerName`, `modelSlug`, and
`priceSource` — absent (`omitempty`) on models that came only from OpenCode
Go/Zen or OpenRouter, so output stays byte-identical with `SOMM_DISCOVERY=off`
or no `opencode` binary on PATH.

#### get_agent_criteria
Read the Gentle AI agent selection criteria.

Parameters:
- `agent`: string (optional) — filter by agent ID

Available agents:
- Orchestrator: `gentle-orchestrator`
- SDD: `sdd-init`, `sdd-onboard`, `sdd-explore`, `sdd-propose`, `sdd-spec`, `sdd-design`, `sdd-tasks`, `sdd-apply`, `sdd-verify`, `sdd-archive`
- Review: `review-risk`, `review-readability`, `review-reliability`, `review-resilience`, `review-refuter`
- Judgment Day: `jd-judge-a`, `jd-judge-b`, `jd-fix-agent`

#### get_model_benchmarks
Search OpenRouter for detailed benchmarks.

Parameters:
- `query`: string — model ID or name

#### estimate_cost
Estimate monthly cost based on model usage patterns.

Parameters:
- `hours_per_day`: number (default: `8`) — average usage hours per day
- `roles`: string[] (optional) — filter specific agent roles

#### compare_models
Compare models side-by-side with benchmarks and pricing.

Parameters:
- `models`: string[] (required) — 2–4 model IDs to compare

#### validate_config
Validate the current configuration and suggest improvements.

Parameters: none

#### export_config
Export the recommended model configuration to `opencode.json`.
Safe: only updates the `model` field.

Parameters:
- `roles`: string[] (optional) — filter specific roles

#### recommend_config
Detect configured providers (OpenCode Go/Zen, OpenRouter, and everything
found via [provider discovery](#provider-discovery)) and recommend the
optimal model per agent role, with the reasoning behind each pick.

Parameters:
- `roles`: string[] (optional) — filter specific agent roles

The response opens with a provider status line per configured provider —
`✅ configured` / ranked, or `⚠️` with a reason when a provider has no
per-token pricing (flat-rate) or no pricing data at all. A role scoped via
`selection.providers` to a provider with no candidates gets a specific
no-match reason instead of falling back to the unscoped pool.

## Development

### Prerequisites

- Go 1.26+

### Commands

```bash
make build         # Build with version info
make test          # Run tests with coverage
make lint          # Run go vet
make install       # Install to GOPATH/bin
make cross-compile # Build for all platforms
make clean         # Remove binaries
make fmt           # Format code
make tidy          # Clean dependencies
make all           # Full pipeline (fmt, tidy, lint, test, build)
```

### Project structure

```
cmd/somm/          # MCP server entry point (serve, setup wizard TUI, chart)
internal/api/      # HTTP client, models, matching, recommendations
internal/guide/    # Embedded guide extraction
internal/profile/  # Role profile schema, presets, resolution
```

### Release

Pushing a `v*` tag triggers GoReleaser via GitHub Actions, which builds and
publishes cross-platform binaries automatically:

```bash
git tag v2.5.0
git push origin v2.5.0
```

## License

MIT © R0LM0 — see [LICENSE](LICENSE).
