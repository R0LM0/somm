package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/R0LM0/somm/internal/api"
	"github.com/R0LM0/somm/internal/guide"
	"github.com/R0LM0/somm/internal/profile"
	"github.com/joho/godotenv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/term"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	// Check for setup subcommand before any setup pre-flight.
	if len(os.Args) > 1 && os.Args[1] == "setup" {
		runSetup()
		return
	}

	skipSetup, remaining := parseSkipSetup(os.Args)
	os.Args = remaining

	if !maybeRunSetup(skipSetup) {
		os.Exit(1)
	}

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "[somm] Fatal error: %v\n", err)
		os.Exit(1)
	}
}

// parseSkipSetup detects --skip-setup and returns the argument list without it
// so the main flag set never sees the flag.
func parseSkipSetup(args []string) (bool, []string) {
	skip := false
	remaining := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--skip-setup" {
			skip = true
			continue
		}
		remaining = append(remaining, arg)
	}
	return skip, remaining
}

func skipSetupFlag(args []string) bool {
	skip, _ := parseSkipSetup(args)
	return skip
}

// maybeRunSetup ensures a required OpenCode API key is available before the MCP
// server starts. It launches the interactive setup wizard when stdin is a TTY,
// and returns false only when configuration is still missing.
func maybeRunSetup(skipSetup bool) bool {
	if skipSetup {
		return true
	}

	loadEnvFromBinaryDir()
	if configReady() {
		return true
	}

	if !isTerminal() {
		fmt.Fprintln(os.Stderr, "[somm] OPENCODE_API_KEY is not configured.")
		fmt.Fprintln(os.Stderr, "Run `somm setup` interactively, set the key in your environment,")
		fmt.Fprintln(os.Stderr, "or pass `--skip-setup` if you will provide `-opencode-api-key`.")
		return false
	}

	runSetup()

	// Re-evaluate after the wizard has written .env.
	loadEnvFromBinaryDir()
	if configReady() {
		fmt.Println("Setup complete! Starting server...")
		return true
	}

	fmt.Fprintln(os.Stderr, "[somm] Setup did not configure OPENCODE_API_KEY.")
	return false
}

func loadEnvFromBinaryDir() {
	if p := binaryEnvPath(); p != "" {
		loadEnvFile(p)
	}
	_ = godotenv.Load()
}

func binaryEnvPath() string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), ".env")
	}
	return ""
}

func loadEnvFile(envPath string) {
	if envMap, err := godotenv.Read(envPath); err == nil {
		for k, v := range envMap {
			if os.Getenv(k) == "" {
				os.Setenv(k, v)
			}
		}
	}
}

func configReady() bool {
	return strings.TrimSpace(os.Getenv("OPENCODE_API_KEY")) != ""
}

func isTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

func run() error {
	// Load .env file from binary's directory (silent ignore if missing)
	if exe, err := os.Executable(); err == nil {
		envPath := filepath.Join(filepath.Dir(exe), ".env")
		if envMap, readErr := godotenv.Read(envPath); readErr == nil {
			for k, v := range envMap {
				if os.Getenv(k) == "" {
					os.Setenv(k, v)
				}
			}
		}
	}
	// Also try current directory as fallback
	_ = godotenv.Load()

	var (
		ocKey       string
		orKey       string
		kimiKey     string
		profilePath string
	)
	// NOTE: API keys passed via CLI flags are visible in process listings (ps aux).
	// Prefer setting them via environment variables or .env file instead.
	flag.StringVar(&ocKey, "opencode-api-key", os.Getenv("OPENCODE_API_KEY"), "OpenCode API key (required)")
	flag.StringVar(&orKey, "openrouter-api-key", os.Getenv("OPENROUTER_API_KEY"), "OpenRouter API key (optional)")
	flag.StringVar(&kimiKey, "kimi-api-key", os.Getenv("KIMI_API_KEY"), "Kimi API key (optional)")
	flag.StringVar(&profilePath, "profile", "", "Path to a role profile YAML file (falls back to SOMM_PROFILE env, ./somm.yaml, XDG config, then the embedded gentle-ai preset)")
	flag.Parse()

	if strings.TrimSpace(ocKey) == "" {
		return errors.New("-opencode-api-key is required")
	}

	// Resolve the active role profile. A file selected by the resolution
	// order that is malformed or fails validation is a fatal error — no
	// silent fallback to the embedded default (role-profiles "Fail-Loud
	// Validation").
	prof, err := profile.Resolve(profilePath)
	if err != nil {
		return fmt.Errorf("resolving profile: %w", err)
	}

	client := api.NewClient(nil, ocKey, orKey, kimiKey)

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "somm",
		Version: version,
		Title:   "Somm — Model Advisor MCP Server",
	}, &mcp.ServerOptions{
		Instructions: "Agent-Model Recommendation Advisor. Fetches available models from OpenCode subscriptions and OpenRouter benchmarks, reads agent selection criteria from the Gentle AI guide, and helps recommend the best model for each agent/sub-agent.",
		InitializedHandler: func(context.Context, *mcp.InitializedRequest) {
			// Log AFTER the JSON-RPC handshake completes — stderr before this point
			// breaks MCP clients.
			slog.Info("somm ready")
		},
	})

	registerListModels(server, client)
	registerGetAgentCriteria(server, prof)
	registerGetModelBenchmarks(server, client)
	registerCompareModels(server, client)
	registerRecommendConfig(server, client, prof)
	registerValidateConfig(server, client, prof)
	registerExportConfig(server, client, prof)
	registerEstimateCost(server, client, prof)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

type listModelsInput struct {
	Subscription string `json:"subscription"`
	Enrich       bool   `json:"enrich"`
}

func listModelsSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"subscription": map[string]any{
				"type":        "string",
				"enum":        []string{"go", "zen", "both"},
				"default":     "both",
				"description": "Which OpenCode subscription(s) to query: go (paid), zen (free), or both",
			},
			"enrich": map[string]any{
				"type":        "boolean",
				"default":     true,
				"description": "Cross-reference with OpenRouter for benchmarks and pricing",
			},
		},
	}
}

func registerListModels(server *mcp.Server, client *api.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:  "list_available_models",
		Title: "List Available Models",
		Description: `Fetch all available AI models from your OpenCode subscriptions (Go and/or Zen),
cross-referenced with OpenRouter benchmarks when available.

Returns for each model:
- ID, name, provider
- Which subscription(s) it belongs to (go, zen, or both)
- Pricing per 1M tokens (input/output) — from OpenRouter
- Context length (max tokens)
- Benchmarks: intelligence_index, coding_index, agentic_index (Artificial Analysis)
- Reasoning: available effort levels (e.g. ["xhigh","high"]) and default — when the model supports reasoning_effort

Use this tool FIRST to understand what models are available before making recommendations.`,
		InputSchema: listModelsSchema(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in listModelsInput) (*mcp.CallToolResult, any, error) {
		models, err := client.ListModels(ctx, in.Subscription, in.Enrich)
		if err != nil {
			return errorResult(err), nil, nil
		}

		summary := buildModelSummary(models)
		text, err := json.MarshalIndent(summary, "", "  ")
		if err != nil {
			return errorResult(err), nil, nil
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(text)}},
		}, nil, nil
	})
}

func buildModelSummary(models []api.EnrichedModel) map[string]any {
	var (
		withReasoning     []api.EnrichedModel
		withEffort        []api.EnrichedModel
		withToggle        []api.EnrichedModel
		withBenchmarks    int
		withoutBenchmarks int
	)
	for _, m := range models {
		if m.Reasoning != nil {
			withReasoning = append(withReasoning, m)
			if len(m.Reasoning.SupportedEfforts) > 0 {
				withEffort = append(withEffort, m)
			} else {
				withToggle = append(withToggle, m)
			}
		}
		if m.Benchmarks.Intelligence != nil {
			withBenchmarks++
		} else {
			withoutBenchmarks++
		}
	}

	return map[string]any{
		"total": len(models),
		"bySubscription": map[string]int{
			"go":   countBySubscription(models, "go"),
			"zen":  countBySubscription(models, "zen"),
			"both": countBySubscription(models, "both"),
		},
		"withBenchmarks":    withBenchmarks,
		"withoutBenchmarks": withoutBenchmarks,
		"reasoning": map[string]int{
			"withEffortLevels": len(withEffort),
			"withToggleOnly":   len(withToggle),
			"none":             len(models) - len(withReasoning),
		},
		"models": models,
	}
}

func countBySubscription(models []api.EnrichedModel, sub string) int {
	count := 0
	for _, m := range models {
		switch sub {
		case "go":
			if m.Subscription == "go" || m.Subscription == "both" {
				count++
			}
		case "zen":
			if m.Subscription == "zen" || m.Subscription == "both" {
				count++
			}
		case "both":
			if m.Subscription == "both" {
				count++
			}
		}
	}
	return count
}

type getAgentCriteriaInput struct {
	Agent string `json:"agent"`
}

func getAgentCriteriaSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"agent": map[string]any{
				"type":        "string",
				"description": "Agent ID to filter by (e.g. 'sdd-apply', 'gentle-orchestrator', 'review-risk', 'jd-judge-a'). Omit to get the full guide.",
			},
		},
	}
}

func registerGetAgentCriteria(server *mcp.Server, prof *profile.Profile) {
	mcp.AddTool(server, &mcp.Tool{
		Name:  "get_agent_criteria",
		Title: "Get Agent Criteria",
		Description: `Read the active profile's agent/role selection criteria.

Each role entry defines:
- What the role does (description)
- Criticidad: how critical the role is to the overall pipeline

Use this to understand WHAT each role needs BEFORE picking a model.
Pass a role ID to get only that role's criteria.

AGENT GROUPS (display recommendations in this order):
1. Orchestrator: gentle-orchestrator
2. SDD agents: sdd-init, sdd-onboard, sdd-explore, sdd-propose, sdd-spec, sdd-design, sdd-tasks, sdd-apply, sdd-verify, sdd-archive
3. Review (4R): review-risk, review-readability, review-reliability, review-resilience, review-refuter
4. Judgment Day: jd-judge-a, jd-judge-b, jd-fix-agent`,
		InputSchema: getAgentCriteriaSchema(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in getAgentCriteriaInput) (*mcp.CallToolResult, any, error) {
		content, err := guide.ExtractFromProfile(prof, in.Agent)
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
			}, nil, nil
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: content}},
		}, nil, nil
	})
}

type getModelBenchmarksInput struct {
	Query string `json:"query"`
}

func getModelBenchmarksSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"query"},
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Model ID or name to search for (e.g. 'deepseek-v4-pro', 'kimi', 'qwen3.7')",
			},
		},
	}
}

func registerGetModelBenchmarks(server *mcp.Server, client *api.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:  "get_model_benchmarks",
		Title: "Get Model Benchmarks",
		Description: `Search OpenRouter for detailed benchmarks and pricing for a specific model.
Use this when you need deeper data on a particular model beyond what list_available_models returns.

Returns: model ID, name, pricing, context length, and artificial_analysis benchmarks.`,
		InputSchema: getModelBenchmarksSchema(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in getModelBenchmarksInput) (*mcp.CallToolResult, any, error) {
		models, err := client.ListORModels(ctx, in.Query)
		if err != nil {
			return errorResult(err), nil, nil
		}

		if len(models) == 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf(`No models found matching %q on OpenRouter.`, in.Query)}},
			}, nil, nil
		}

		results := make([]map[string]any, 0, len(models))
		for _, m := range models {
			contextLength := m.ContextLength
			if contextLength == nil && m.TopProvider != nil {
				contextLength = m.TopProvider.ContextLength
			}

			var pricing map[string]float64
			if m.Pricing != nil {
				pricing = map[string]float64{
					"input_per_1M":  api.ParseMoney(m.Pricing.Prompt) * 1_000_000,
					"output_per_1M": api.ParseMoney(m.Pricing.Completion) * 1_000_000,
				}
			}

			var benchmarks map[string]*float64
			if m.Benchmarks != nil && m.Benchmarks.ArtificialAnalysis != nil {
				aa := m.Benchmarks.ArtificialAnalysis
				benchmarks = map[string]*float64{
					"intelligence_index": aa.IntelligenceIndex,
					"coding_index":       aa.CodingIndex,
					"agentic_index":      aa.AgenticIndex,
				}
			}

			results = append(results, map[string]any{
				"id":             m.ID,
				"name":           m.Name,
				"context_length": contextLength,
				"pricing":        pricing,
				"benchmarks":     benchmarks,
			})
		}

		text, err := json.MarshalIndent(map[string]any{
			"query":   in.Query,
			"count":   len(results),
			"results": results,
		}, "", "  ")
		if err != nil {
			return errorResult(err), nil, nil
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(text)}},
		}, nil, nil
	})
}

type compareModelsInput struct {
	ModelIDs []string `json:"model_ids"`
}

func compareModelsSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"model_ids"},
		"properties": map[string]any{
			"model_ids": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"minItems":    2,
				"maxItems":    4,
				"description": "2-4 OpenRouter model IDs to compare (e.g. [\"deepseek/deepseek-v4-pro\", \"moonshotai/kimi-k3\"])",
			},
		},
	}
}

func registerCompareModels(server *mcp.Server, client *api.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:  "compare_models",
		Title: "Compare Models",
		Description: `Compare 2-4 models side-by-side with benchmarks and pricing from OpenRouter.

Returns a comparison table with:
- Intelligence, Coding, and Agentic indices
- Input and output pricing per 1M tokens
- Context length
- Supported reasoning effort levels
- Winner recommendations by category (best coding, best price, best balance, best intelligence, best agentic)

Use this to evaluate tradeoffs between specific models before recommending one for a role.`,
		InputSchema: compareModelsSchema(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in compareModelsInput) (*mcp.CallToolResult, any, error) {
		result, err := api.CompareModels(ctx, client, in.ModelIDs)
		if err != nil {
			return errorResult(err), nil, nil
		}

		text := api.FormatComparison(result)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: text}},
		}, nil, nil
	})
}

func errorResult(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: "Error: " + err.Error()}},
	}
}

type estimateCostInput struct {
	HoursPerDay float64  `json:"hours_per_day"`
	Roles       []string `json:"roles"`
}

func estimateCostSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"hours_per_day": map[string]any{
				"type":        "number",
				"default":     8,
				"description": "Average hours of usage per day (default: 8)",
			},
			"roles": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Filter specific agent roles (e.g. [\"sdd-apply\", \"sdd-verify\"]). Omit for all roles.",
			},
		},
	}
}

func registerEstimateCost(server *mcp.Server, client *api.Client, prof *profile.Profile) {
	mcp.AddTool(server, &mcp.Tool{
		Name:  "estimate_cost",
		Title: "Estimate Monthly Cost",
		Description: `Estimates monthly API costs based on your recommended model configuration.

For each agent role, it:
1. Gets the recommended model and its pricing
2. Estimates tokens per session based on agent type (e.g. orchestrator uses ~50K input, sdd-apply uses ~100K input)
3. Calculates monthly cost: tokens × sessions × price

Assumes ~22 working days/month with 1 hour sessions.
Shows breakdown by agent and by provider.

Use this to understand the financial impact before applying a configuration.`,
		InputSchema: estimateCostSchema(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in estimateCostInput) (*mcp.CallToolResult, any, error) {
		result, err := api.EstimateCost(ctx, client, prof, in.HoursPerDay, in.Roles)
		if err != nil {
			return errorResult(err), nil, nil
		}

		text := api.FormatCostResult(result)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: text}},
		}, nil, nil
	})
}

type validateConfigInput struct{}

func validateConfigSchema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func registerValidateConfig(server *mcp.Server, client *api.Client, prof *profile.Profile) {
	mcp.AddTool(server, &mcp.Tool{
		Name:  "validate_config",
		Title: "Validate Configuration",
		Description: `Analyzes your current MCP server configuration and checks if it's optimal.

Detects configured API providers, lists available models, and evaluates
whether the current setup provides the best quality/price for each agent role.
Returns a configuration score and concrete improvement suggestions.

Use this to quickly assess if your setup is complete or if adding more
API keys would improve recommendations.`,
		InputSchema: validateConfigSchema(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in validateConfigInput) (*mcp.CallToolResult, any, error) {
		result, err := api.ValidateConfig(ctx, client, prof)
		if err != nil {
			return errorResult(err), nil, nil
		}

		text := api.FormatValidation(result)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: text}},
		}, nil, nil
	})
}

type exportConfigInput struct {
	Roles []string `json:"roles"`
}

func exportConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"roles": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Filter specific roles (optional)",
			},
		},
	}
}

func registerExportConfig(server *mcp.Server, client *api.Client, prof *profile.Profile) {
	mcp.AddTool(server, &mcp.Tool{
		Name:  "export_config",
		Title: "Export Configuration",
		Description: `Export recommended model configuration to opencode.json format (SAFE: only updates model field).

Reads your existing opencode.json, generates model recommendations,
and returns an updated JSON with ONLY the "model" field changed per agent.
All other config (prompts, tools, permissions) is preserved.

This tool NEVER writes to disk. It returns the JSON for you to review and apply manually.

Use this after recommend_config to get a ready-to-paste opencode.json.`,
		InputSchema: exportConfigSchema(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in exportConfigInput) (*mcp.CallToolResult, any, error) {
		result, err := api.ExportConfig(ctx, client, prof, "", in.Roles)
		if err != nil {
			return errorResult(err), nil, nil
		}

		text := api.FormatExportResult(result)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: text}},
		}, nil, nil
	})
}

type recommendConfigInput struct {
	Roles []string `json:"roles"`
}

func recommendConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"roles": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Filter specific agent roles (e.g. [\"sdd-apply\", \"sdd-verify\"]). Omit for all roles.",
			},
		},
	}
}

func registerRecommendConfig(server *mcp.Server, client *api.Client, prof *profile.Profile) {
	mcp.AddTool(server, &mcp.Tool{
		Name:  "recommend_config",
		Title: "Recommend Configuration",
		Description: `Recommends the best model for each agent role based on your configured providers and model benchmarks.

Detects which API keys are configured (OPENCODE_API_KEY, OPENROUTER_API_KEY),
lists available models from your subscriptions, cross-references with OpenRouter
benchmarks, and recommends the optimal model per agent role using quality/price ratio.

Agent roles evaluated:
1. Orchestrator (CRÍTICO) - routing, instruction following
2. SDD agents: init, onboard, explore, propose, spec, design, tasks, apply, verify, archive
3. Review (4R): risk, readability, reliability, resilience, refuter
4. Judgment Day: judge-a, judge-b, fix-agent

Pass specific roles to filter recommendations. Omit for the complete set.`,
		InputSchema: recommendConfigSchema(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in recommendConfigInput) (*mcp.CallToolResult, any, error) {
		providers, recs, err := api.RecommendConfig(ctx, client, prof, in.Roles)
		if err != nil {
			return errorResult(err), nil, nil
		}

		text := api.FormatRecommendations(providers, recs)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: text}},
		}, nil, nil
	})
}
