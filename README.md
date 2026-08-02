# Model Advisor MCP Server

Agent-Model Recommendation Advisor for Gentle AI. Fetches available models from OpenCode subscriptions and OpenRouter benchmarks, reads agent selection criteria, and recommends the best model for each agent.

[![Tests](https://github.com/R0LM0/sub-aware-agent-model-advisor-go/actions/workflows/test.yml/badge.svg)](https://github.com/R0LM0/sub-aware-agent-model-advisor-go/actions/workflows/test.yml)
[![Release](https://github.com/R0LM0/sub-aware-agent-model-advisor-go/actions/workflows/release.yml/badge.svg)](https://github.com/R0LM0/sub-aware-agent-model-advisor-go/releases)

## Features

- List available models from OpenCode Go/Zen subscriptions
- Cross-reference with OpenRouter benchmarks and pricing
- Read agent selection criteria from Gentle AI guide
- Search specific model benchmarks
- Automatic .env configuration loading

## Installation

### From Release (Recommended)
Download the latest binary from [Releases](https://github.com/R0LM0/sub-aware-agent-model-advisor-go/releases)

### From Source
git clone https://github.com/R0LM0/sub-aware-agent-model-advisor-go.git
cd sub-aware-agent-model-advisor-go
make install

### Using Go
go install github.com/AlonsoSG0/model-advisor-mcp/cmd/server@latest

## Configuration

### Environment Variables
| Variable | Required | Description |
|----------|----------|-------------|
| OPENCODE_API_KEY | Yes | OpenCode Go/Zen subscription key |
| OPENROUTER_API_KEY | No | OpenRouter API key for benchmarks |

### .env File
Create a .env file next to the binary:
OPENCODE_API_KEY=sk-your-key-here
OPENROUTER_API_KEY=sk-or-your-key-here

### Flags
./model-advisor -opencode-api-key sk-xxx -openrouter-api-key sk-or-xxx

## Usage

### With OpenCode
Add to your opencode.json:
"mcp": {
  "model-advisor": {
    "command": ["path/to/model-advisor"],
    "enabled": true,
    "type": "local"
  }
}

### Available Tools

#### list_available_models
Fetch all available AI models from your subscriptions.

Parameters:
- subscription: "go" | "zen" | "both" (default: "both")
- enrich: boolean (default: true) — cross-reference with OpenRouter

#### get_agent_criteria
Read the Gentle AI agent selection criteria.

Parameters:
- agent: string (optional) — filter by agent ID

Available agents:
- Orchestrator: gentle-orchestrator
- SDD: sdd-init, sdd-onboard, sdd-explore, sdd-propose, sdd-spec, sdd-design, sdd-tasks, sdd-apply, sdd-verify, sdd-archive
- Review: review-risk, review-readability, review-reliability, review-resilience, review-refuter
- Judgment Day: jd-judge-a, jd-judge-b, jd-fix-agent

#### get_model_benchmarks
Search OpenRouter for detailed benchmarks.

Parameters:
- query: string — model ID or name

## Development

### Prerequisites
- Go 1.26+

### Commands
make build        # Build with version info
make test         # Run tests with coverage
make lint         # Run go vet
make install      # Install to GOPATH/bin
make cross-compile # Build for all platforms
make clean        # Remove binaries
make fmt          # Format code
make tidy         # Clean dependencies
make all          # Full pipeline (fmt, tidy, lint, test, build)

### Project Structure
cmd/server/        # MCP server entry point
internal/api/      # HTTP client, models, matching
internal/guide/    # Embedded guide extraction

### Testing
go test ./... -v

### Release
git tag v1.0.0
git push origin v1.0.0

## License

MIT
