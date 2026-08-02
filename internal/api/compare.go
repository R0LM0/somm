package api

import (
	"context"
	"fmt"
	"strings"
)

// ComparisonResult holds the side-by-side comparison of 2-4 models.
type ComparisonResult struct {
	Models  []ModelComparison `json:"models"`
	Winners map[string]string `json:"winners"`
}

// ModelComparison holds extracted data for a single model in the comparison.
type ModelComparison struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Intelligence  *float64 `json:"intelligence"`
	Coding        *float64 `json:"coding"`
	Agentic       *float64 `json:"agentic"`
	PriceInput    *float64 `json:"price_input"`
	PriceOutput   *float64 `json:"price_output"`
	ContextLength *int64   `json:"context_length"`
	Reasoning     []string `json:"reasoning"`
}

// CompareModels fetches OpenRouter data for the given model IDs and returns
// a side-by-side comparison. It requires 2-4 model IDs.
func CompareModels(ctx context.Context, client *Client, modelIDs []string) (*ComparisonResult, error) {
	if len(modelIDs) < 2 || len(modelIDs) > 4 {
		return nil, fmt.Errorf("compare_models requires 2-4 model IDs, got %d", len(modelIDs))
	}

	orModels, err := client.fetchOpenRouter(ctx, orURL)
	if err != nil {
		return nil, fmt.Errorf("fetching OpenRouter models: %w", err)
	}

	var comparisons []ModelComparison
	for _, id := range modelIDs {
		mc, err := extractComparison(id, orModels)
		if err != nil {
			return nil, err
		}
		comparisons = append(comparisons, *mc)
	}

	winners := determineWinners(comparisons)

	return &ComparisonResult{
		Models:  comparisons,
		Winners: winners,
	}, nil
}

// extractComparison finds an OR model by ID (case-insensitive) and extracts
// benchmark and pricing data into a ModelComparison.
func extractComparison(targetID string, orModels []ORModel) (*ModelComparison, error) {
	lower := strings.ToLower(targetID)

	var match *ORModel
	for i := range orModels {
		if strings.ToLower(orModels[i].ID) == lower {
			match = &orModels[i]
			break
		}
	}

	if match == nil {
		return nil, fmt.Errorf("model %q not found on OpenRouter", targetID)
	}

	mc := ModelComparison{
		ID:   match.ID,
		Name: match.Name,
	}

	// Extract context length.
	if match.ContextLength != nil {
		mc.ContextLength = match.ContextLength
	} else if match.TopProvider != nil && match.TopProvider.ContextLength != nil {
		mc.ContextLength = match.TopProvider.ContextLength
	}

	// Extract pricing (per 1M tokens).
	if match.Pricing != nil {
		inputPer1M := ParseMoney(match.Pricing.Prompt) * 1_000_000
		outputPer1M := ParseMoney(match.Pricing.Completion) * 1_000_000
		mc.PriceInput = &inputPer1M
		mc.PriceOutput = &outputPer1M
	}

	// Extract benchmarks.
	if match.Benchmarks != nil && match.Benchmarks.ArtificialAnalysis != nil {
		aa := match.Benchmarks.ArtificialAnalysis
		mc.Intelligence = aa.IntelligenceIndex
		mc.Coding = aa.CodingIndex
		mc.Agentic = aa.AgenticIndex
	}

	// Extract reasoning efforts.
	if match.Reasoning != nil && len(match.Reasoning.SupportedEfforts) > 0 {
		mc.Reasoning = match.Reasoning.SupportedEfforts
	}

	return &mc, nil
}

// determineWinners finds the best model in each category.
func determineWinners(comparisons []ModelComparison) map[string]string {
	winners := make(map[string]string)

	// Best coding: highest coding index.
	bestCoding := findHighest(comparisons, func(mc ModelComparison) *float64 {
		return mc.Coding
	})
	if bestCoding != nil {
		winners["best_coding"] = fmt.Sprintf("%s (%.1f)", bestCoding.Name, *bestCoding.Coding)
	}

	// Best price: lowest input price.
	bestPrice := findLowest(comparisons, func(mc ModelComparison) *float64 {
		return mc.PriceInput
	})
	if bestPrice != nil {
		winners["best_price"] = fmt.Sprintf("%s ($%.3f/M)", bestPrice.Name, *bestPrice.PriceInput)
	}

	// Best balance: highest (coding / input_price). Only when both values exist.
	bestBalance := findBestBalance(comparisons)
	if bestBalance != nil {
		score := *bestBalance.Coding / *bestBalance.PriceInput
		winners["best_balance"] = fmt.Sprintf("%s (%.1f coding / $%.3f = %.1f points/$)", bestBalance.Name, *bestBalance.Coding, *bestBalance.PriceInput, score)
	}

	// Best intelligence: highest intelligence index.
	bestIntel := findHighest(comparisons, func(mc ModelComparison) *float64 {
		return mc.Intelligence
	})
	if bestIntel != nil {
		winners["best_intelligence"] = fmt.Sprintf("%s (%.1f)", bestIntel.Name, *bestIntel.Intelligence)
	}

	// Best agentic: highest agentic index.
	bestAgentic := findHighest(comparisons, func(mc ModelComparison) *float64 {
		return mc.Agentic
	})
	if bestAgentic != nil {
		winners["best_agentic"] = fmt.Sprintf("%s (%.1f)", bestAgentic.Name, *bestAgentic.Agentic)
	}

	return winners
}

// findHighest returns the model with the highest value from the accessor.
func findHighest(comparisons []ModelComparison, accessor func(ModelComparison) *float64) *ModelComparison {
	var best *ModelComparison
	var bestVal float64
	for i := range comparisons {
		v := accessor(comparisons[i])
		if v == nil {
			continue
		}
		if best == nil || *v > bestVal {
			best = &comparisons[i]
			bestVal = *v
		}
	}
	return best
}

// findLowest returns the model with the lowest value from the accessor.
func findLowest(comparisons []ModelComparison, accessor func(ModelComparison) *float64) *ModelComparison {
	var best *ModelComparison
	var bestVal float64
	for i := range comparisons {
		v := accessor(comparisons[i])
		if v == nil {
			continue
		}
		if best == nil || *v < bestVal {
			best = &comparisons[i]
			bestVal = *v
		}
	}
	return best
}

// findBestBalance returns the model with the highest coding/price ratio.
func findBestBalance(comparisons []ModelComparison) *ModelComparison {
	var best *ModelComparison
	var bestScore float64
	for i := range comparisons {
		mc := &comparisons[i]
		if mc.Coding == nil || mc.PriceInput == nil || *mc.PriceInput == 0 {
			continue
		}
		score := *mc.Coding / *mc.PriceInput
		if best == nil || score > bestScore {
			best = mc
			bestScore = score
		}
	}
	return best
}

// FormatComparison renders the ComparisonResult as a markdown table.
func FormatComparison(result *ComparisonResult) string {
	if len(result.Models) == 0 {
		return "No models to compare."
	}

	var sb strings.Builder

	// Header line.
	names := make([]string, len(result.Models))
	for i, m := range result.Models {
		names[i] = m.Name
	}
	sb.WriteString(fmt.Sprintf("Comparacion: %s\n\n", strings.Join(names, " vs ")))

	// Table header.
	sb.WriteString("| Metric |")
	for _, m := range result.Models {
		sb.WriteString(fmt.Sprintf(" %s |", m.Name))
	}
	sb.WriteString("\n|--------|")
	for range result.Models {
		sb.WriteString("---------|")
	}
	sb.WriteString("\n")

	// Intelligence row.
	sb.WriteString("| Intelligence |")
	for _, m := range result.Models {
		if m.Intelligence != nil {
			sb.WriteString(fmt.Sprintf(" %.1f |", *m.Intelligence))
		} else {
			sb.WriteString(" - |")
		}
	}
	sb.WriteString("\n")

	// Coding row.
	sb.WriteString("| Coding |")
	for _, m := range result.Models {
		if m.Coding != nil {
			sb.WriteString(fmt.Sprintf(" %.1f |", *m.Coding))
		} else {
			sb.WriteString(" - |")
		}
	}
	sb.WriteString("\n")

	// Agentic row.
	sb.WriteString("| Agentic |")
	for _, m := range result.Models {
		if m.Agentic != nil {
			sb.WriteString(fmt.Sprintf(" %.1f |", *m.Agentic))
		} else {
			sb.WriteString(" - |")
		}
	}
	sb.WriteString("\n")

	// Price input row.
	sb.WriteString("| Precio input |")
	for _, m := range result.Models {
		if m.PriceInput != nil {
			sb.WriteString(fmt.Sprintf(" $%.3f/M |", *m.PriceInput))
		} else {
			sb.WriteString(" - |")
		}
	}
	sb.WriteString("\n")

	// Price output row.
	sb.WriteString("| Precio output |")
	for _, m := range result.Models {
		if m.PriceOutput != nil {
			sb.WriteString(fmt.Sprintf(" $%.3f/M |", *m.PriceOutput))
		} else {
			sb.WriteString(" - |")
		}
	}
	sb.WriteString("\n")

	// Context row.
	sb.WriteString("| Contexto |")
	for _, m := range result.Models {
		if m.ContextLength != nil {
			sb.WriteString(fmt.Sprintf(" %d |", *m.ContextLength))
		} else {
			sb.WriteString(" - |")
		}
	}
	sb.WriteString("\n")

	// Reasoning row.
	sb.WriteString("| Reasoning |")
	for _, m := range result.Models {
		if len(m.Reasoning) > 0 {
			sb.WriteString(fmt.Sprintf(" %s |", strings.Join(m.Reasoning, ", ")))
		} else {
			sb.WriteString(" - |")
		}
	}
	sb.WriteString("\n\n")

	// Winners.
	if len(result.Winners) > 0 {
		if v, ok := result.Winners["best_coding"]; ok {
			sb.WriteString(fmt.Sprintf("Mejor para coding: %s\n", v))
		}
		if v, ok := result.Winners["best_intelligence"]; ok {
			sb.WriteString(fmt.Sprintf("Mejor para intelligence: %s\n", v))
		}
		if v, ok := result.Winners["best_agentic"]; ok {
			sb.WriteString(fmt.Sprintf("Mejor para agentic: %s\n", v))
		}
		if v, ok := result.Winners["best_price"]; ok {
			sb.WriteString(fmt.Sprintf("Mejor para price: %s\n", v))
		}
		if v, ok := result.Winners["best_balance"]; ok {
			sb.WriteString(fmt.Sprintf("Mejor balance: %s\n", v))
		}
	}

	return sb.String()
}
