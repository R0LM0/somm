package api

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// TokenUsage describes estimated tokens per session for an agent type.
type TokenUsage struct {
	Input  int `json:"input"`
	Output int `json:"output"`
}

// CostEstimate is the estimated cost for a single agent role.
type CostEstimate struct {
	Agent           string     `json:"agent"`
	Model           string     `json:"model"`
	Provider        string     `json:"provider"`
	TokensPerSession TokenUsage `json:"tokensPerSession"`
	CostPerSession   float64    `json:"costPerSession"`
	MonthlyCost      float64    `json:"monthlyCost"`
	SessionsPerMonth  int        `json:"sessionsPerMonth"`
}

// CostResult is the full cost estimation output.
type CostResult struct {
	Estimates    []CostEstimate    `json:"estimates"`
	TotalMonthly float64           `json:"totalMonthly"`
	ByProvider   map[string]float64 `json:"byProvider"`
	HoursPerDay  float64           `json:"hoursPerDay"`
	DaysPerMonth int               `json:"daysPerMonth"`
}

// defaultTokenUsage returns token estimates for a given agent role.
func defaultTokenUsage(agentID string) TokenUsage {
	switch agentID {
	case "orchestrator":
		return TokenUsage{Input: 50_000, Output: 10_000}
	case "sdd-apply":
		return TokenUsage{Input: 100_000, Output: 50_000}
	case "sdd-verify":
		return TokenUsage{Input: 80_000, Output: 20_000}
	case "sdd-explore":
		return TokenUsage{Input: 60_000, Output: 15_000}
	default:
		return TokenUsage{Input: 30_000, Output: 10_000}
	}
}

// EstimateCost computes monthly cost estimates for agent roles.
// hoursPerDay is daily usage, roles filters which agents to include.
func EstimateCost(ctx context.Context, client *Client, hoursPerDay float64, roles []string) (*CostResult, error) {
	if hoursPerDay <= 0 {
		hoursPerDay = 8
	}

	// Sessions per month: hours/day × 22 working days.
	// Each session assumed to last ~1 hour of active interaction.
	sessionsPerMonth := int(hoursPerDay * 22)

	_, recs, err := RecommendConfig(ctx, client, roles)
	if err != nil {
		return nil, fmt.Errorf("getting recommendations: %w", err)
	}

	result := &CostResult{
		Estimates:    make([]CostEstimate, 0, len(recs)),
		ByProvider:   make(map[string]float64),
		HoursPerDay:  hoursPerDay,
		DaysPerMonth: 22,
	}

	for _, rec := range recs {
		if rec.Model == "" {
			continue
		}

		tokens := defaultTokenUsage(rec.Agent)

		// Cost per session = (input_tokens / 1M × price_input) + (output_tokens / 1M × price_output)
		inputCost := float64(tokens.Input) / 1_000_000 * rec.PriceInput
		outputCost := float64(tokens.Output) / 1_000_000 * rec.PriceOutput
		costPerSession := inputCost + outputCost
		monthlyCost := costPerSession * float64(sessionsPerMonth)

		est := CostEstimate{
			Agent:            rec.Agent,
			Model:            rec.Model,
			Provider:         rec.Provider,
			TokensPerSession: tokens,
			CostPerSession:   costPerSession,
			MonthlyCost:      monthlyCost,
			SessionsPerMonth: sessionsPerMonth,
		}

		result.Estimates = append(result.Estimates, est)
		result.TotalMonthly += monthlyCost
		result.ByProvider[rec.Provider] += monthlyCost
	}

	// Sort by monthly cost descending.
	sort.Slice(result.Estimates, func(i, j int) bool {
		return result.Estimates[i].MonthlyCost > result.Estimates[j].MonthlyCost
	})

	return result, nil
}

// FormatCostResult returns a human-readable cost estimate for MCP output.
func FormatCostResult(result *CostResult) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Estimado mensual (%.0f horas/día, %d días/mes):\n\n",
		result.HoursPerDay, result.DaysPerMonth))

	sb.WriteString("Por agente:\n")
	for _, est := range result.Estimates {
		sb.WriteString(fmt.Sprintf("- %s (%s): $%.2f/mes\n",
			est.Agent, est.Model, est.MonthlyCost))
		sb.WriteString(fmt.Sprintf("  %dK input + %dK output × %d sesiones\n",
			est.TokensPerSession.Input/1000,
			est.TokensPerSession.Output/1000,
			est.SessionsPerMonth))
	}

	sb.WriteString(fmt.Sprintf("\nTotal estimado: ~$%.2f/mes\n", result.TotalMonthly))

	sb.WriteString("\nDesglose por provider:\n")
	// Sort providers by cost descending.
	type providerCost struct {
		name string
		cost float64
	}
	providers := make([]providerCost, 0, len(result.ByProvider))
	for name, cost := range result.ByProvider {
		providers = append(providers, providerCost{name, cost})
	}
	sort.Slice(providers, func(i, j int) bool {
		return providers[i].cost > providers[j].cost
	})
	for _, p := range providers {
		if p.cost == 0 {
			sb.WriteString(fmt.Sprintf("- %s: $0 (incluido en suscripción)\n", p.name))
		} else {
			sb.WriteString(fmt.Sprintf("- %s: ~$%.2f/mes\n", p.name, p.cost))
		}
	}

	return sb.String()
}
