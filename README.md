# Model Advisor MCP Server

Agent-Model Recommendation Advisor for Gentle AI. Fetches available models from OpenCode subscriptions and OpenRouter benchmarks, reads agent selection criteria, and recommends the best model for each agent.

[![Tests](https://github.com/R0LM0/sub-aware-agent-model-advisor-go/actions/workflows/test.yml/badge.svg)](https://github.com/R0LM0/sub-aware-agent-model-advisor-go/actions/workflows/test.yml)
[![Release](https://github.com/R0LM0/sub-aware-agent-model-advisor-go/actions/workflows/release.yml/badge.svg)](https://github.com/R0LM0/sub-aware-agent-model-advisor-go/releases)

## Features

- List available models from OpenCode Go/Zen subscriptions
- Cross-reference with OpenRouter benchmarks and pricing
- Read agent selection criteria from Gentle AI guide
- Search specific model benchmarks
- **Recommend optimal model configuration per agent role**
- **Estimate monthly costs by agent**
- **Compare models side-by-side**
- **Validate current configuration**
- **Export safe config to opencode.json**
- Automatic .env configuration loading
- HTTP timeout, retry, and graceful degradation

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

#### estimate_cost
Estimate monthly cost based on model usage patterns.

Parameters:
- hours_per_day: number (default: 8) — average usage hours per day
- roles: string[] (optional) — filter specific agent roles

Example output:
```
Estimado mensual (8 horas/día, 22 días/mes):

Por agente:
- orchestrator (DeepSeek V4 Pro): $3.20/mes
- sdd-apply (Kimi K3): $12.50/mes
- sdd-verify (GPT-5.5-Pro): $45.00/mes

Total estimado: ~$60.70/mes

Desglose por provider:
- OpenCode Go: $0 (incluido en suscripción)
- Kimi: ~$20.90/mes
- OpenRouter: ~$39.80/mes
```

#### compare_models
Compare models side-by-side with benchmarks and pricing.

Parameters:
- models: string[] (required) — 2-4 model IDs to compare

Example output:
```
Comparación: DeepSeek V4 Pro vs Kimi K3 vs GPT-5.6 Luna

| Métrica | DeepSeek V4 Pro | Kimi K3 | GPT-5.6 Luna |
|---------|-----------------|---------|--------------|
| Intelligence | 44.3 | 57.1 | 51.2 |
| Coding | 59.4 | 76.2 | 71.4 |
| Precio input | $0.435/M | $0.95/M | $1.25/M |

Mejor para coding: Kimi K3 (76.2)
Mejor para price: DeepSeek V4 Pro ($0.435/M)
Mejor balance: Kimi K3 (76.2 / $0.95 = 80.2 points/$)
```

#### validate_config
Validate current configuration and suggest improvements.

Parameters: none

Example output:
```
Análisis de configuración actual:

✅ OpenCode Go: Configurado (45 modelos)
✅ OpenRouter: Configurado (200+ modelos)
❌ Kimi: No detectado (recomendado para coding)

Score de configuración: 7/10

Mejoras sugeridas:
1. Agregar KIMI_API_KEY para mejorar sdd-apply (+15% coding)
2. Agregar OPENAI_API_KEY para mejorar sdd-verify (+20% razonamiento)
```

#### export_config
Export recommended model configuration to opencode.json (SAFE: only updates model field).

Parameters:
- roles: string[] (optional) — filter specific roles

Example output:
```
Cambios a aplicar:
- sdd-apply: kimi-for-coding/kimi-for-coding → kimi-for-coding/kimi-k3
- sdd-verify: openai/gpt-5.6-luna → openai/gpt-5.5-pro

⚠️ Solo actualiza el campo "model". Tu config de Gentle AI NO se modifica.
```

#### recommend_config
Detect configured providers and recommend optimal model per agent role.

Parameters:
- roles: string[] (optional) — filter specific agent roles

Example output:
```
Tu configuración actual:
✅ OpenCode Go (API key configurada)
✅ OpenRouter (API key configurada)

Recomendación por agente:

orchestrator → DeepSeek V4 Pro
  Calidad: 44.3 intelligence | Precio: $0.435/M input
  Provider: OpenCode Go
  Por qué: Mejor relación calidad/precio para routing

sdd-apply → Kimi K3
  Calidad: 76.2 coding | Precio: $0.95/M input
  Provider: Kimi Code (vía OpenRouter)
  Por qué: Tenés acceso K3, es el mejor coding de Kimi
```

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
git tag v1.2.0
git push origin v1.2.0

## License

MIT
