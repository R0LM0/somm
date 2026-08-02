package api

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// BuildOpenRouterHeaders returns the request headers for an OpenRouter call.
// When apiKey is non-empty an Authorization Bearer header is added; otherwise
// no Authorization header is present.
func BuildOpenRouterHeaders(apiKey string) http.Header {
	h := make(http.Header)
	if apiKey != "" {
		h.Set("Authorization", "Bearer "+apiKey)
	}
	return h
}

// MatchOR finds the best OpenRouter model for an OpenCode model ID.
// Matching precedence is:
//  1. case-insensitive exact ID match;
//  2. ordered provider alias replacement, first match wins;
//  3. substring match (single result wins, multiple results return shortest ID).
func MatchOR(ocID string, orModels []ORModel) *ORModel {
	if len(orModels) == 0 || ocID == "" {
		return nil
	}

	needle := strings.ToLower(ocID)

	// 1. Exact match.
	for i := range orModels {
		if strings.ToLower(orModels[i].ID) == needle {
			return &orModels[i]
		}
	}

	// 2. Provider alias (ordered slice).
	for _, alias := range PROVIDER_ALIASES {
		if strings.HasPrefix(ocID, alias.Prefix) {
			aliased := strings.Replace(ocID, alias.Prefix, alias.Replacement, 1)
			aliasedLower := strings.ToLower(aliased)
			for i := range orModels {
				if strings.ToLower(orModels[i].ID) == aliasedLower {
					return &orModels[i]
				}
			}
		}
	}

	// 3. Substring match.
	lower := needle
	subs := make([]ORModel, 0)
	for i := range orModels {
		o := strings.ToLower(orModels[i].ID)
		if strings.Contains(o, lower) || strings.Contains(lower, o) {
			subs = append(subs, orModels[i])
		}
	}
	if len(subs) == 0 {
		return nil
	}
	if len(subs) == 1 {
		return &subs[0]
	}
	sort.Slice(subs, func(i, j int) bool {
		return len(subs[i].ID) < len(subs[j].ID)
	})
	return &subs[0]
}

// enrichWithOpenRouter mutates the enriched models with data from OpenRouter.
// Only matched models are updated; unmatched models keep their zero/nil values.
func enrichWithOpenRouter(enriched []EnrichedModel, orModels []ORModel) {
	for i := range enriched {
		match := MatchOR(enriched[i].OCID, orModels)
		if match == nil {
			continue
		}
		model := &enriched[i]
		model.ORID = &match.ID
		model.ORName = &match.Name

		if match.Pricing != nil {
			model.Pricing = &Money{
				Prompt:     parseMoney(match.Pricing.Prompt),
				Completion: parseMoney(match.Pricing.Completion),
			}
		}

		if match.ContextLength != nil {
			model.ContextLength = match.ContextLength
		} else if match.TopProvider != nil && match.TopProvider.ContextLength != nil {
			model.ContextLength = match.TopProvider.ContextLength
		}

		if match.Benchmarks != nil && match.Benchmarks.ArtificialAnalysis != nil {
			aa := match.Benchmarks.ArtificialAnalysis
			model.Benchmarks = ModelBenchmarks{
				Intelligence: aa.IntelligenceIndex,
				Coding:       aa.CodingIndex,
				Agentic:      aa.AgenticIndex,
			}
		}

		if match.Reasoning != nil {
			mandatory := false
			if match.Reasoning.Mandatory != nil {
				mandatory = *match.Reasoning.Mandatory
			}
			defaultEnabled := !mandatory
			if match.Reasoning.DefaultEnabled != nil {
				defaultEnabled = *match.Reasoning.DefaultEnabled
			}
			model.Reasoning = &ModelReasoning{
				SupportedEfforts: match.Reasoning.SupportedEfforts,
				DefaultEffort:    match.Reasoning.DefaultEffort,
				Mandatory:        mandatory,
				DefaultEnabled:   defaultEnabled,
			}
		}
	}
}

func parseMoney(s string) float64 {
	if s == "" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}
