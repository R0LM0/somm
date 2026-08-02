package api

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
)

// AgentRole defines a role that needs a model recommendation.
type AgentRole struct {
	ID          string
	Name        string
	Criticidad  string
	Criteria    string
	BestMetric  string // "intelligence", "coding", "agentic", or "cheapest"
}

// Recommendation is the output for a single agent role.
type Recommendation struct {
	Agent       string   `json:"agent"`
	Criticidad  string   `json:"criticidad"`
	Model       string   `json:"model"`
	OCID        string   `json:"ocId"`
	Provider    string   `json:"provider"`
	PriceInput  float64  `json:"priceInput"`
	PriceOutput float64  `json:"priceOutput"`
	Intelligence *float64 `json:"intelligence,omitempty"`
	Coding      *float64 `json:"coding,omitempty"`
	Agentic     *float64 `json:"agentic,omitempty"`
	ContextLen  *int64   `json:"contextLength,omitempty"`
	Reason      string   `json:"reason"`
}

// ProviderStatus describes which providers are available.
type ProviderStatus struct {
	Name       string `json:"name"`
	Configured bool   `json:"configured"`
}

// AllAgentRoles returns the complete list of agent roles with their criteria.
func AllAgentRoles() []AgentRole {
	return []AgentRole{
		{ID: "orchestrator", Name: "Orchestrator", Criticidad: "CRÍTICO", Criteria: "instruction following + contexto largo", BestMetric: "intelligence"},
		{ID: "sdd-init", Name: "SDD Init", Criticidad: "MEDIA", Criteria: "contexto masivo + bajo costo", BestMetric: "cheapest"},
		{ID: "sdd-onboard", Name: "SDD Onboard", Criticidad: "MEDIA", Criteria: "contexto largo + razonamiento moderado", BestMetric: "intelligence"},
		{ID: "sdd-explore", Name: "SDD Explore", Criticidad: "ALTA", Criteria: "análisis multi-paso + razonamiento sobre código", BestMetric: "agentic"},
		{ID: "sdd-propose", Name: "SDD Propose", Criticidad: "CRÍTICO", Criteria: "máximo razonamiento arquitectónico", BestMetric: "intelligence"},
		{ID: "sdd-spec", Name: "SDD Spec", Criticidad: "ALTA", Criteria: "escritura técnica + output estructurado", BestMetric: "intelligence"},
		{ID: "sdd-design", Name: "SDD Design", Criticidad: "MEDIA", Criteria: "especialización visual/multimodal", BestMetric: "intelligence"},
		{ID: "sdd-tasks", Name: "SDD Tasks", Criticidad: "BAJA", Criteria: "velocidad + formato + costo mínimo", BestMetric: "cheapest"},
		{ID: "sdd-apply", Name: "SDD Apply", Criticidad: "CRÍTICO", Criteria: "máxima capacidad de coding", BestMetric: "coding"},
		{ID: "sdd-verify", Name: "SDD Verify", Criticidad: "CRÍTICO", Criteria: "máximo razonamiento + different from apply", BestMetric: "intelligence"},
		{ID: "sdd-archive", Name: "SDD Archive", Criticidad: "BAJA", Criteria: "costo mínimo absoluto", BestMetric: "cheapest"},
		{ID: "review-risk", Name: "Review Risk", Criticidad: "CRÍTICO", Criteria: "conocimiento seguridad + evidencia concreta", BestMetric: "intelligence"},
		{ID: "review-readability", Name: "Review Readability", Criticidad: "MEDIA", Criteria: "juicio de estructura + mantenibilidad", BestMetric: "intelligence"},
		{ID: "review-reliability", Name: "Review Reliability", Criticidad: "ALTA", Criteria: "behavior-first + edge cases", BestMetric: "intelligence"},
		{ID: "review-resilience", Name: "Review Resilience", Criticidad: "ALTA", Criteria: "patrones de fallo + SRE mindset", BestMetric: "intelligence"},
		{ID: "review-refuter", Name: "Review Refuter", Criticidad: "ALTA", Criteria: "rigor lógico + independencia", BestMetric: "intelligence"},
		{ID: "jd-judge-a", Name: "JD Judge A", Criticidad: "CRÍTICO", Criteria: "razonamiento adversarial máximo", BestMetric: "intelligence"},
		{ID: "jd-judge-b", Name: "JD Judge B", Criticidad: "CRÍTICO", Criteria: "razonamiento adversarial máximo", BestMetric: "intelligence"},
		{ID: "jd-fix-agent", Name: "JD Fix Agent", Criticidad: "ALTA", Criteria: "instruction following preciso + coding", BestMetric: "coding"},
	}
}

// filterRoles returns only the roles whose IDs are in the filter list.
// If filter is empty, all roles are returned.
func filterRoles(all []AgentRole, filter []string) []AgentRole {
	if len(filter) == 0 {
		return all
	}
	wanted := make(map[string]bool, len(filter))
	for _, f := range filter {
		wanted[f] = true
	}
	var result []AgentRole
	for _, r := range all {
		if wanted[r.ID] {
			result = append(result, r)
		}
	}
	return result
}

// scoredModel pairs a model with its score for a specific metric.
type scoredModel struct {
	model EnrichedModel
	score float64
	price float64 // input price per 1M tokens
}

// RecommendConfig computes the best model for each agent role based on
// available models and their benchmarks/pricing. It returns the list of
// provider statuses and recommendations sorted by criticidad.
func RecommendConfig(ctx context.Context, client *Client, roles []string) ([]ProviderStatus, []Recommendation, error) {
	providers := []ProviderStatus{
		{Name: "OpenCode Go", Configured: client.OCAPIKey != ""},
		{Name: "OpenRouter", Configured: client.ORAPIKey != ""},
	}

	if client.OCAPIKey == "" {
		return providers, nil, ErrOCKeyMissing
	}

	// Fetch OpenCode models (both subscriptions) enriched with OpenRouter benchmarks.
	models, err := client.ListModels(ctx, "both", true)
	if err != nil {
		return providers, nil, fmt.Errorf("fetching models: %w", err)
	}

	if len(models) == 0 {
		return providers, nil, fmt.Errorf("no models available from configured providers")
	}

	agentRoles := filterRoles(AllAgentRoles(), roles)
	if len(agentRoles) == 0 {
		return providers, nil, fmt.Errorf("no matching agent roles for filter: %v", roles)
	}

	// Track which models are already assigned.
	assignmentCount := make(map[string]int)

	// Score models for each role and pick the best.
	recs := make([]Recommendation, 0, len(agentRoles))
	for _, role := range agentRoles {
		best := findBestModel(models, role, assignmentCount)
		if best == nil {
			recs = append(recs, Recommendation{
				Agent:      role.ID,
				Criticidad: role.Criticidad,
				Reason:     "No model with benchmarks available for this role",
			})
			continue
		}

		assignmentCount[best.model.OCID]++

		rec := Recommendation{
			Agent:       role.ID,
			Criticidad:  role.Criticidad,
			Model:       best.model.OCName,
			OCID:        best.model.OCID,
			Provider:    deriveProvider(best.model.OCID),
			PriceInput:  best.model.Pricing.Prompt * 1_000_000,
			PriceOutput: best.model.Pricing.Completion * 1_000_000,
			Intelligence: best.model.Benchmarks.Intelligence,
			Coding:      best.model.Benchmarks.Coding,
			Agentic:     best.model.Benchmarks.Agentic,
			ContextLen:  best.model.ContextLength,
			Reason:      buildReason(role, best),
		}
		recs = append(recs, rec)
	}

	// Sort by criticidad priority: CRÍTICO > ALTA > MEDIA > BAJA.
	critOrder := map[string]int{"CRÍTICO": 0, "ALTA": 1, "MEDIA": 2, "BAJA": 3}
	sort.Slice(recs, func(i, j int) bool {
		return critOrder[recs[i].Criticidad] < critOrder[recs[j].Criticidad]
	})

	return providers, recs, nil
}

// findBestModel selects the best model for a given role, respecting the
// max-2-assignments-per-model rule. When all models are already assigned twice,
// it relaxes the constraint to avoid returning nil.
func findBestModel(models []EnrichedModel, role AgentRole, used map[string]int) *scoredModel {
	// First pass: try with max-2 constraint.
	candidates := collectCandidates(models, role, used, 2)
	if len(candidates) == 0 {
		// Second pass: relax constraint to max-3 if no candidates found.
		candidates = collectCandidates(models, role, used, 3)
	}
	if len(candidates) == 0 {
		return nil
	}

	// For "cheapest" metric, sort by price first (ascending), then by score as tiebreaker.
	if role.BestMetric == "cheapest" {
		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].price != candidates[j].price {
				return candidates[i].price < candidates[j].price
			}
			return candidates[i].score > candidates[j].score
		})
		return &candidates[0]
	}

	// For other metrics, compute quality/price ratio and sort by it.
	type ratioed struct {
		scoredModel
		ratio float64
	}
	ratioedCandidates := make([]ratioed, len(candidates))
	for i, c := range candidates {
		r := 0.0
		if c.price > 0 {
			r = c.score / c.price
		}
		ratioedCandidates[i] = ratioed{scoredModel: c, ratio: r}
	}

	sort.Slice(ratioedCandidates, func(i, j int) bool {
		if ratioedCandidates[i].ratio != ratioedCandidates[j].ratio {
			return ratioedCandidates[i].ratio > ratioedCandidates[j].ratio
		}
		// Tiebreak: higher raw score wins.
		return ratioedCandidates[i].score > ratioedCandidates[j].score
	})

	return &ratioedCandidates[0].scoredModel
}

// collectCandidates builds scored models that match the role's metric and
// haven't exceeded the maxUses threshold.
func collectCandidates(models []EnrichedModel, role AgentRole, used map[string]int, maxUses int) []scoredModel {
	candidates := make([]scoredModel, 0)

	for _, m := range models {
		// Skip models already at the usage limit.
		if used[m.OCID] >= maxUses {
			continue
		}

		// Skip models without pricing.
		if m.Pricing == nil || m.Pricing.Prompt == 0 {
			continue
		}

		price := m.Pricing.Prompt * 1_000_000 // per 1M tokens

		var score float64
		switch role.BestMetric {
		case "intelligence":
			if m.Benchmarks.Intelligence != nil {
				score = *m.Benchmarks.Intelligence
			} else {
				continue
			}
		case "coding":
			if m.Benchmarks.Coding != nil {
				score = *m.Benchmarks.Coding
			} else {
				continue
			}
		case "agentic":
			if m.Benchmarks.Agentic != nil {
				score = *m.Benchmarks.Agentic
			} else {
				continue
			}
		case "cheapest":
			if m.Benchmarks.Intelligence != nil {
				score = *m.Benchmarks.Intelligence
			} else {
				score = 50.0
			}
		}

		candidates = append(candidates, scoredModel{
			model: m,
			score: score,
			price: price,
		})
	}

	return candidates
}

// buildReason generates a human-readable reason for the recommendation.
func buildReason(role AgentRole, best *scoredModel) string {
	m := best.model
	price := m.Pricing.Prompt * 1_000_000

	var metric string
	switch role.BestMetric {
	case "intelligence":
		if m.Benchmarks.Intelligence != nil {
			metric = fmt.Sprintf("%.1f intelligence", *m.Benchmarks.Intelligence)
		}
	case "coding":
		if m.Benchmarks.Coding != nil {
			metric = fmt.Sprintf("%.1f coding", *m.Benchmarks.Coding)
		}
	case "agentic":
		if m.Benchmarks.Agentic != nil {
			metric = fmt.Sprintf("%.1f agentic", *m.Benchmarks.Agentic)
		}
	case "cheapest":
		if m.Benchmarks.Intelligence != nil {
			metric = fmt.Sprintf("intelligence %.1f", *m.Benchmarks.Intelligence)
		} else {
			metric = "sin benchmarks"
		}
	}

	ctxInfo := ""
	if m.ContextLength != nil {
		ctxK := *m.ContextLength / 1000
		ctxInfo = fmt.Sprintf(", %dK ctx", ctxK)
	}

	subInfo := ""
	if m.Subscription == "both" {
		subInfo = " (Go + Zen)"
	} else if m.Subscription == "go" {
		subInfo = " (Go)"
	} else {
		subInfo = " (Zen)"
	}

	return fmt.Sprintf("Mejor relación calidad/precio: %s, $%.3f/M input%s%s — %s", metric, price, ctxInfo, subInfo, role.Criteria)
}

// FormatRecommendations returns a formatted text block for MCP output.
func FormatRecommendations(providers []ProviderStatus, recs []Recommendation) string {
	var sb strings.Builder

	sb.WriteString("Tu configuración actual:\n")
	for _, p := range providers {
		if p.Configured {
			sb.WriteString(fmt.Sprintf("✅ %s (API key configurada)\n", p.Name))
		} else {
			sb.WriteString(fmt.Sprintf("❌ %s (no configurado)\n", p.Name))
		}
	}

	sb.WriteString("\nRecomendación por agente (ordenada por prioridad):\n\n")

	for _, r := range recs {
		if r.Model == "" {
			sb.WriteString(fmt.Sprintf("%s → ⚠️  Sin recomendación\n", r.Agent))
			sb.WriteString(fmt.Sprintf("  %s\n\n", r.Reason))
			continue
		}

		sb.WriteString(fmt.Sprintf("%s → %s\n", r.Agent, r.Model))

		// Metrics line.
		metrics := []string{}
		if r.Intelligence != nil {
			metrics = append(metrics, fmt.Sprintf("%.1f intelligence", *r.Intelligence))
		}
		if r.Coding != nil {
			metrics = append(metrics, fmt.Sprintf("%.1f coding", *r.Coding))
		}
		if r.Agentic != nil {
			metrics = append(metrics, fmt.Sprintf("%.1f agentic", *r.Agentic))
		}
		if len(metrics) > 0 {
			sb.WriteString(fmt.Sprintf("  Calidad: %s | Precio: $%.3f/M input\n", strings.Join(metrics, ", "), r.PriceInput))
		} else {
			sb.WriteString(fmt.Sprintf("  Precio: $%.3f/M input (sin benchmarks)\n", r.PriceInput))
		}

		sb.WriteString(fmt.Sprintf("  Provider: %s\n", r.Provider))
		sb.WriteString(fmt.Sprintf("  Por qué: %s\n\n", r.Reason))
	}

	return sb.String()
}

// safeDiv returns 0 if divisor is 0 to avoid division by zero.
func safeDiv(a, b float64) float64 {
	if math.Abs(b) < 1e-9 {
		return 0
	}
	return a / b
}
