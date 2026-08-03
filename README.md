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

- List available models from OpenCode Go/Zen subscriptions
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

## Installation

### Using Go

```bash
go install github.com/R0LM0/somm/cmd/somm@latest
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

### Environment variables

| Variable | Required | Description |
|----------|----------|-------------|
| `OPENCODE_API_KEY` | Yes | OpenCode Go/Zen subscription key |
| `OPENROUTER_API_KEY` | No | OpenRouter API key for benchmarks |
| `SOMM_PROFILE` | No | Path to a role profile YAML file (see [Role profiles](#role-profiles)) |

### .env file

Create a `.env` file next to the binary:

```
OPENCODE_API_KEY=sk-your-key-here
OPENROUTER_API_KEY=sk-or-your-key-here
```

### Flags

```bash
somm -opencode-api-key sk-xxx -openrouter-api-key sk-or-xxx -profile ./somm.yaml
```

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
roles:
  - id: my-agent
    description: "needs strong coding + a context floor"
    criticidad: "CRÍTICO"
    weights:
      coding: 0.7
      intelligence: 0.3
    max_input_price: 5.0
    requires: ["reasoning"]
```

## Usage

### Setup wizard

```bash
somm setup
```

Detects your OpenCode config and wires Somm into `opencode.json` automatically.

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
Fetch all available AI models from your subscriptions.

Parameters:
- `subscription`: `"go" | "zen" | "both"` (default: `"both"`)
- `enrich`: boolean (default: `true`) — cross-reference with OpenRouter

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
Detect configured providers and recommend the optimal model per agent role,
with the reasoning behind each pick.

Parameters:
- `roles`: string[] (optional) — filter specific agent roles

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
cmd/somm/          # MCP server entry point (serve + setup wizard)
internal/api/      # HTTP client, models, matching
internal/guide/    # Embedded guide extraction
```

### Release

```bash
git tag v2.0.0
git push origin v2.0.0
```

## License

MIT © R0LM0 — see [LICENSE](LICENSE).
