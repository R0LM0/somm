package api

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/R0LM0/somm/internal/profile"
)

const maxScorePoints = 10

// Recommendation is the output for a single agent role.
type Recommendation struct {
	Agent        string   `json:"agent"`
	Criticidad   string   `json:"criticidad"`
	Model        string   `json:"model"`
	OCID         string   `json:"ocId"`
	Provider     string   `json:"provider"`
	PriceInput   float64  `json:"priceInput"`
	PriceOutput  float64  `json:"priceOutput"`
	Intelligence *float64 `json:"intelligence,omitempty"`
	Coding       *float64 `json:"coding,omitempty"`
	Agentic      *float64 `json:"agentic,omitempty"`
	ContextLen   *int64   `json:"contextLength,omitempty"`
	Reason       string   `json:"reason"`
}

// ProviderStatus describes which providers are available.
type ProviderStatus struct {
	Name       string `json:"name"`
	Configured bool   `json:"configured"`
}

// filterRoles returns only the roles whose IDs are in the filter list.
// If filter is empty, all roles are returned.
func filterRoles(all []profile.Role, filter []string) []profile.Role {
	if len(filter) == 0 {
		return all
	}
	wanted := make(map[string]bool, len(filter))
	for _, f := range filter {
		wanted[f] = true
	}
	var result []profile.Role
	for _, r := range all {
		if wanted[r.ID] {
			result = append(result, r)
		}
	}
	return result
}

// scoredModel pairs a candidate model with its computed scores for a
// specific role.
//
//   - qraw holds the raw weighted sum (Σ weight_m * raw_m) for weighted
//     roles, or the intelligence-based tiebreak value (50.0 fallback when
//     null) for empty-weights price-minimizing roles. The quality/price
//     ratio and winner tiebreak always use qraw — see design Decision 1.
//   - qnorm holds the min-max normalized weighted sum, populated only for
//     roles weighting >=2 metrics. It is an internal quality comparison
//     value only and never feeds the ratio (design Decision 2).
//   - price is the input price per 1M tokens.
type scoredModel struct {
	model EnrichedModel
	qraw  float64
	qnorm float64
	price float64
}

// RecommendConfig computes the best model for each role in the active
// Profile based on available models and their benchmarks/pricing. It
// returns the list of provider statuses and recommendations sorted by
// criticidad.
func RecommendConfig(ctx context.Context, client *Client, prof *profile.Profile, roleFilter []string) ([]ProviderStatus, []Recommendation, error) {
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

	roles := filterRoles(prof.Roles, roleFilter)
	if len(roles) == 0 {
		return providers, nil, fmt.Errorf("no matching agent roles for filter: %v", roleFilter)
	}

	// Track which models are already assigned, and which provider family
	// was assigned to each role (for exclude_family_of).
	assignmentCount := make(map[string]int)
	assignedFamily := make(map[string]string)

	// Score models for each role and pick the best.
	recs := make([]Recommendation, 0, len(roles))
	for _, role := range roles {
		best := findBestModel(models, role, assignmentCount, assignedFamily)
		if best == nil {
			recs = append(recs, Recommendation{
				Agent:      role.ID,
				Criticidad: role.Criticidad,
				Reason:     "No model with benchmarks available for this role",
			})
			continue
		}

		assignmentCount[best.model.OCID]++
		assignedFamily[role.ID] = deriveProvider(best.model.OCID)

		var priceInput, priceOutput float64
		if best.model.Pricing != nil {
			priceInput = best.model.Pricing.Prompt * 1_000_000
			priceOutput = best.model.Pricing.Completion * 1_000_000
		}

		rec := Recommendation{
			Agent:        role.ID,
			Criticidad:   role.Criticidad,
			Model:        best.model.OCName,
			OCID:         best.model.OCID,
			Provider:     deriveProvider(best.model.OCID),
			PriceInput:   priceInput,
			PriceOutput:  priceOutput,
			Intelligence: best.model.Benchmarks.Intelligence,
			Coding:       best.model.Benchmarks.Coding,
			Agentic:      best.model.Benchmarks.Agentic,
			ContextLen:   best.model.ContextLength,
			Reason:       buildReason(role, best),
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
// max-2-assignments-per-model rule. When all models are already assigned
// twice, it relaxes the constraint to max-3 to avoid returning nil. Hard
// constraints (min_context, max_input_price, requires, exclude_family_of)
// are never relaxed.
func findBestModel(models []EnrichedModel, role profile.Role, used map[string]int, assignedFamily map[string]string) *scoredModel {
	// First pass: try with max-2 constraint.
	candidates := collectCandidates(models, role, used, 2, assignedFamily)
	if len(candidates) == 0 {
		// Second pass: relax the assignment cap to max-3 if no candidates found.
		candidates = collectCandidates(models, role, used, 3, assignedFamily)
	}
	if len(candidates) == 0 {
		return nil
	}

	// Empty weights: price-minimizing objective. Sort by price ascending,
	// tiebreak by intelligence descending (qraw holds that tiebreak value).
	if len(role.Weights) == 0 {
		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].price != candidates[j].price {
				return candidates[i].price < candidates[j].price
			}
			return candidates[i].qraw > candidates[j].qraw
		})
		return &candidates[0]
	}

	// Multi-metric roles also compute Qnorm as an internal comparison value.
	// It never feeds the ratio below (design Decision 1 & 2).
	computeNormalized(role, candidates)

	// Quality/price ratio ALWAYS uses the raw weighted sum, never a
	// normalized value — normalize-then-ratio reorders winners (design
	// Decision 1).
	sort.Slice(candidates, func(i, j int) bool {
		ri, rj := qualityPriceRatio(candidates[i]), qualityPriceRatio(candidates[j])
		if ri != rj {
			return ri > rj
		}
		// Tiebreak: higher raw score wins.
		return candidates[i].qraw > candidates[j].qraw
	})

	return &candidates[0]
}

// qualityPriceRatio returns Qraw / price for a candidate, or 0 when price
// is not positive.
func qualityPriceRatio(c scoredModel) float64 {
	if c.price <= 0 {
		return 0
	}
	return c.qraw / c.price
}

// collectCandidates builds scored models for a role that pass the hard
// constraint pre-filter, in this exact order (weighted-scoring spec, "Hard
// Constraint Pre-Filter"):
//  1. skip models with no usable pricing
//  2. skip models already at maxUses
//  3. apply min_context, max_input_price, requires, exclude_family_of
//  4. for weighted roles, skip candidates with a nil value on any
//     positively-weighted metric
func collectCandidates(models []EnrichedModel, role profile.Role, used map[string]int, maxUses int, assignedFamily map[string]string) []scoredModel {
	candidates := make([]scoredModel, 0)

	for _, m := range models {
		// (1) Skip models without usable pricing.
		if m.Pricing == nil || m.Pricing.Prompt == 0 {
			continue
		}
		price := m.Pricing.Prompt * 1_000_000 // per 1M tokens

		// (2) Skip models already at the usage limit.
		if used[m.OCID] >= maxUses {
			continue
		}

		// (3) Hard constraints — never relaxed.
		if role.MinContext != nil {
			if m.ContextLength == nil || *m.ContextLength < *role.MinContext {
				continue
			}
		}
		if role.MaxInputPrice != nil && price > *role.MaxInputPrice {
			continue
		}
		if !satisfiesRequires(m, role.Requires) {
			continue
		}
		if role.ExcludeFamilyOf != "" {
			if fam, ok := assignedFamily[role.ExcludeFamilyOf]; ok && deriveProvider(m.OCID) == fam {
				continue
			}
		}

		if len(role.Weights) == 0 {
			// Price-minimizing objective: qraw holds the intelligence
			// tiebreak value, 50.0 fallback when null (mirrors today's
			// "cheapest" behavior exactly).
			score := 50.0
			if m.Benchmarks.Intelligence != nil {
				score = *m.Benchmarks.Intelligence
			}
			candidates = append(candidates, scoredModel{model: m, qraw: score, price: price})
			continue
		}

		// (4) Weighted roles: skip candidates missing a positively-weighted metric.
		qraw := 0.0
		skip := false
		for metric, w := range role.Weights {
			if w <= 0 {
				continue
			}
			v := metricValue(m, metric)
			if v == nil {
				skip = true
				break
			}
			qraw += w * (*v)
		}
		if skip {
			continue
		}

		candidates = append(candidates, scoredModel{model: m, qraw: qraw, price: price})
	}

	return candidates
}

// satisfiesRequires checks that a candidate model satisfies every
// capability token in requires. Only "reasoning" is currently implemented
// (matched against model.Reasoning != nil); any other token currently fails
// the constraint rather than being silently ignored, since the token set is
// expected to grow (role-profiles spec, "Capability Token Support").
func satisfiesRequires(m EnrichedModel, requires []string) bool {
	for _, token := range requires {
		switch token {
		case "reasoning":
			if m.Reasoning == nil {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// metricValue returns the raw benchmark value for a metric key, or nil if
// the metric is unknown or the model has no value for it.
func metricValue(m EnrichedModel, metric string) *float64 {
	switch metric {
	case profile.MetricIntelligence:
		return m.Benchmarks.Intelligence
	case profile.MetricCoding:
		return m.Benchmarks.Coding
	case profile.MetricAgentic:
		return m.Benchmarks.Agentic
	default:
		return nil
	}
}

// computeNormalized fills in candidates[i].qnorm with the min-max
// normalized weighted sum, but only when the role weights two or more
// metrics positively. Normalization is computed per metric over the given
// (already constraint-filtered) candidate set; when the range for a metric
// is zero (including a single-candidate set), norm is 1.0 for all
// candidates on that metric — never a division by zero (weighted-scoring
// spec, "Multi-Metric Normalization").
func computeNormalized(role profile.Role, candidates []scoredModel) {
	positiveMetrics := make([]string, 0, len(role.Weights))
	for metric, w := range role.Weights {
		if w > 0 {
			positiveMetrics = append(positiveMetrics, metric)
		}
	}
	if len(positiveMetrics) < 2 {
		return
	}

	for _, metric := range positiveMetrics {
		weight := role.Weights[metric]
		min, max := math.Inf(1), math.Inf(-1)
		values := make([]float64, len(candidates))
		for i, c := range candidates {
			v := metricValue(c.model, metric)
			if v == nil {
				// collectCandidates step 4 already excludes candidates
				// missing a positively-weighted metric; this is defensive.
				continue
			}
			values[i] = *v
			if *v < min {
				min = *v
			}
			if *v > max {
				max = *v
			}
		}

		rangeV := max - min
		for i, val := range values {
			norm := 1.0
			if rangeV != 0 {
				norm = (val - min) / rangeV
			}
			candidates[i].qnorm += weight * norm
		}
	}
}

// dominantMetric returns the metric key with the highest positive weight,
// breaking ties by a fixed precedence order (intelligence, coding, agentic)
// for determinism.
func dominantMetric(weights map[string]float64) string {
	order := []string{profile.MetricIntelligence, profile.MetricCoding, profile.MetricAgentic}
	best := ""
	bestWeight := math.Inf(-1)
	for _, metric := range order {
		w, ok := weights[metric]
		if !ok {
			continue
		}
		if w > bestWeight {
			bestWeight = w
			best = metric
		}
	}
	return best
}

// buildReason generates a human-readable reason for the recommendation.
func buildReason(role profile.Role, best *scoredModel) string {
	m := best.model
	price := 0.0
	if m.Pricing != nil {
		price = m.Pricing.Prompt * 1_000_000
	}

	var metric string
	if len(role.Weights) == 0 {
		// Price-minimizing objective (mirrors today's "cheapest" text exactly).
		if m.Benchmarks.Intelligence != nil {
			metric = fmt.Sprintf("intelligence %.1f", *m.Benchmarks.Intelligence)
		} else {
			metric = "sin benchmarks"
		}
	} else if dom := dominantMetric(role.Weights); dom != "" {
		if v := metricValue(m, dom); v != nil {
			metric = fmt.Sprintf("%.1f %s", *v, dom)
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

	return fmt.Sprintf("Mejor relación calidad/precio: %s, $%.3f/M input%s%s — %s", metric, price, ctxInfo, subInfo, role.Description)
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
