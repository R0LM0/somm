package api

import (
	"context"
	"fmt"
	"strings"
)

// ValidationResult contains the full validation output.
type ValidationResult struct {
	Providers    []ProviderStatusDetail `json:"providers"`
	AgentChecks  []AgentCheck           `json:"agentChecks"`
	Score        int                    `json:"score"`
	Improvements []string               `json:"improvements"`
}

// ProviderStatusDetail extends ProviderStatus with model count.
type ProviderStatusDetail struct {
	Name       string `json:"name"`
	Configured bool   `json:"configured"`
	Models     int    `json:"models"`
}

// AgentCheck reports validation status for a single agent role.
type AgentCheck struct {
	Agent           string `json:"agent"`
	CurrentModel    string `json:"currentModel"`
	Status          string `json:"status"` // "optimal", "warning", "error"
	Suggestion      string `json:"suggestion"`
	ImprovementNote string `json:"improvementNote,omitempty"`
}

// ValidateConfig analyzes the current configuration and returns a validation result.
func ValidateConfig(ctx context.Context, client *Client) (*ValidationResult, error) {
	result := &ValidationResult{}

	// 1. Check provider status.
_ocConfigured := client.OCAPIKey != ""
	_orConfigured := client.ORAPIKey != ""

	result.Providers = append(result.Providers, ProviderStatusDetail{
		Name:       "OpenCode Go",
		Configured: _ocConfigured,
	})
	result.Providers = append(result.Providers, ProviderStatusDetail{
		Name:       "OpenRouter",
		Configured: _orConfigured,
	})

	// 2. Fetch models if OpenCode is configured.
	var models []EnrichedModel
	if _ocConfigured {
		var err error
		models, err = client.ListModels(ctx, "both", true)
		if err != nil {
			return nil, fmt.Errorf("fetching models: %w", err)
		}
	}

	// Update model counts per provider.
	goCount := 0
	zenCount := 0
	for _, m := range models {
		switch m.Subscription {
		case "go":
			goCount++
		case "zen":
			zenCount++
		case "both":
			goCount++
			zenCount++
		}
	}
	result.Providers[0].Models = goCount + zenCount

	// 3. Get recommendations to compare against.
	_, recs, err := RecommendConfig(ctx, client, nil)
	if err != nil {
		return nil, fmt.Errorf("generating recommendations: %w", err)
	}

	// 4. For each agent, check if recommended model is available and optimal.
ModelIndex := make(map[string]EnrichedModel, len(models))
for _, m := range models {
	ModelIndex[m.OCID] = m
}

score := 0
maxScore := len(recs)

for _, rec := range recs {
	check := AgentCheck{
		Agent:        rec.Agent,
		CurrentModel: rec.Model,
	}

	if rec.Model == "" {
		check.Status = "error"
		check.Suggestion = "No hay modelo disponible con benchmarks para este agente"
	} else if rec.Model != "" {
		m, ok := ModelIndex[rec.OCID]
		if !ok {
			check.Status = "warning"
			check.Suggestion = fmt.Sprintf("Modelo %s recomendado pero no encontrado en modelos disponibles", rec.Model)
		} else {
			// Check if there's a clearly better option for critical roles.
			check.Status = "optimal"
			check.Suggestion = rec.Reason

			if rec.Criticidad == "CRÍTICO" {
				// For critical roles, check if a higher-scoring model exists.
				bestAlt := findBestAlternative(models, m, rec)
				if bestAlt != nil {
					check.ImprovementNote = fmt.Sprintf(
						"Alternativa: %s (mejor relación calidad/precio)",
						bestAlt.OCName,
					)
				}
			}
		}
	}

	if check.Status == "optimal" {
		score++
	}

	result.AgentChecks = append(result.AgentChecks, check)
}

	// 5. Calculate score.
	if maxScore > 0 {
		result.Score = score * 10 / maxScore
	}

	// 6. Generate improvement suggestions.
	result.Improvements = generateImprovements(result.Providers, models, recs)

	return result, nil
}

// findBestAlternative looks for a better model for a critical role.
// Returns nil if the current recommendation is already the best.
func findBestAlternative(models []EnrichedModel, current EnrichedModel, rec Recommendation) *EnrichedModel {
	// For coding roles, check if a higher-coding model exists.
	if rec.Coding != nil {
		bestScore := *rec.Coding
		for _, m := range models {
			if m.OCID == current.OCID {
				continue
			}
			if m.Benchmarks.Coding != nil && *m.Benchmarks.Coding > bestScore {
				if m.Pricing != nil && m.Pricing.Prompt > 0 {
					// Only suggest if price is within 2x.
					currentPrice := current.Pricing.Prompt
					if currentPrice > 0 && m.Pricing.Prompt <= currentPrice*2 {
						return &m
					}
				}
			}
		}
	}

	// For intelligence roles, check if a higher-intelligence model exists.
	if rec.Intelligence != nil {
		bestScore := *rec.Intelligence
		for _, m := range models {
			if m.OCID == current.OCID {
				continue
			}
			if m.Benchmarks.Intelligence != nil && *m.Benchmarks.Intelligence > bestScore {
				if m.Pricing != nil && m.Pricing.Prompt > 0 {
					currentPrice := current.Pricing.Prompt
					if currentPrice > 0 && m.Pricing.Prompt <= currentPrice*2 {
						return &m
					}
				}
			}
		}
	}

	return nil
}

// generateImprovements produces concrete improvement suggestions.
func generateImprovements(providers []ProviderStatusDetail, models []EnrichedModel, recs []Recommendation) []string {
	var improvements []string

	// Check if OpenRouter is missing — it provides benchmarks.
	_orConfigured := false
	for _, p := range providers {
		if p.Name == "OpenRouter" {
			_orConfigured = p.Configured
		}
	}
	if !_orConfigured {
		improvements = append(improvements,
			"Agregar OPENROUTER_API_KEY para habilitar benchmarks y comparación de precios")
	}

	// Check for models without benchmarks.
	_withBenchmarks := 0
	_withoutBenchmarks := 0
	for _, m := range models {
		if m.Benchmarks.Intelligence != nil {
			_withBenchmarks++
		} else {
			_withoutBenchmarks++
		}
	}
	if _withoutBenchmarks > 0 {
		improvements = append(improvements,
			fmt.Sprintf("%d modelos sin benchmarks — agregar OPENROUTER_API_KEY para enriquecer datos",
				_withoutBenchmarks))
	}

	// Check for critical roles without optimal models.
	for _, r := range recs {
		if r.Criticidad == "CRÍTICO" && r.Model == "" {
			improvements = append(improvements,
				fmt.Sprintf("Rol %s sin modelo asignado — verificar configuración de API keys", r.Agent))
		}
	}

	// Suggest specific providers based on agent needs.
	_needsCoding := false
	for _, r := range recs {
		if r.Coding != nil && *r.Coding > 0 {
			_needsCoding = true
			break
		}
	}
	if _needsCoding {
		_hasKimi := false
		for _, m := range models {
			if strings.Contains(strings.ToLower(m.OCID), "kimi") {
				_hasKimi = true
				break
			}
		}
		if !_hasKimi {
			improvements = append(improvements,
				"Agregar KIMI_API_KEY para mejorar capacidades de coding")
		}
	}

	return improvements
}

// FormatValidation returns a formatted text block for MCP output.
func FormatValidation(result *ValidationResult) string {
	var sb strings.Builder

	sb.WriteString("Análisis de configuración actual:\n\n")

	for _, p := range result.Providers {
		if p.Configured {
			sb.WriteString(fmt.Sprintf("✅ %s: Configurado (%d modelos)\n", p.Name, p.Models))
		} else {
			sb.WriteString(fmt.Sprintf("❌ %s: No detectado\n", p.Name))
		}
	}

	sb.WriteString("\nAnálisis por agente:\n\n")

	for _, check := range result.AgentChecks {
		var icon string
		switch check.Status {
		case "optimal":
			icon = "✅"
		case "warning":
			icon = "⚠️"
		case "error":
			icon = "❌"
		default:
			icon = "❓"
		}

		if check.CurrentModel != "" {
			sb.WriteString(fmt.Sprintf("%s (%s):\n", check.Agent, check.CurrentModel))
		} else {
			sb.WriteString(fmt.Sprintf("%s:\n", check.Agent))
		}

		sb.WriteString(fmt.Sprintf("  %s %s\n", icon, check.Suggestion))

		if check.ImprovementNote != "" {
			sb.WriteString(fmt.Sprintf("  💡 %s\n", check.ImprovementNote))
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("Score de configuración: %d/10\n\n", result.Score))

	if len(result.Improvements) > 0 {
		sb.WriteString("Mejoras sugeridas:\n")
		for i, imp := range result.Improvements {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, imp))
		}
	} else {
		sb.WriteString("✅ Configuración óptima — no se requieren mejoras\n")
	}

	return sb.String()
}
