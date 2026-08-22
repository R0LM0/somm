package api

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/R0LM0/somm/v2/internal/profile"
	"github.com/R0LM0/somm/v2/internal/profile/plans"
)

const maxScorePoints = 10

// hSat is the fixed quota saturation constant (plan-quota-currency spec,
// Requirement: Frequency Weighting): the point past which extra quota
// headroom stops differentiating candidates. Without this cap,
// frequency_weight is a per-role constant that cancels out of every
// Qraw/denominator comparison and never changes the winner within a role —
// the cap is what makes "frequency" load-bearing (design Decision 2).
const hSat = 200.0

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
//
// Ranked and ExcludedReason are populated for providers surfaced by local
// `opencode` CLI discovery (design D12): Ranked is false when every model
// from that provider was excluded from ranking (e.g. a flat-rate,
// OAuth-gated $0-price subscription), and ExcludedReason names why —
// distinguishable from the generic no-pricing-data reason (multi-provider-
// catalog spec, Requirement: Zero-Price Discovered Models Are Excluded With
// a Distinguishable Reason). Both fields default to their zero value
// (Ranked true is set explicitly wherever exclusion isn't tracked at this
// granularity) so existing output stays byte-identical when discovery is
// absent (design Migration/Rollout).
type ProviderStatus struct {
	Name           string `json:"name"`
	Configured     bool   `json:"configured"`
	Ranked         bool   `json:"ranked"`
	ExcludedReason string `json:"excludedReason,omitempty"`
}

// flatRateReason is surfaced for a discovered model with cost.input==0 and
// cost.output==0 (OAuth/subscription-gated, no per-token billing). Such a
// model is excluded from ranking via collectCandidates' existing nil/
// zero-price gate, but the exclusion must be explainable rather than a
// silent omission (design D12; multi-provider-catalog spec, Requirement:
// Zero-Price Discovered Models Are Excluded With a Distinguishable Reason).
const flatRateReason = "flat-rate subscription, no usage-cap ranking available"

// noPricingReason is the generic exclusion reason for a discovered provider
// whose models carry no usable pricing data at all — distinguishable from
// flatRateReason per the same requirement.
const noPricingReason = "no pricing data available"

// discoveredProviderStatuses builds one ProviderStatus entry per distinct
// provider surfaced by local `opencode` CLI discovery (design D12): the
// limitation being surfaced (a $0-price subscription can't be ranked) is
// provider-level, not per-model or per-role, so one entry states it once.
// Only models with a non-empty ProviderID are considered — HTTP-sourced OC
// Go/Zen models keep ProviderID empty (Phase 4's byte-identical-output
// guarantee) and already have their own hardcoded entries in
// RecommendConfig, so they are excluded here to avoid duplicate entries.
// Entries are ordered by ProviderID for deterministic output.
func discoveredProviderStatuses(models []EnrichedModel) []ProviderStatus {
	type agg struct {
		name      string
		anyPriced bool
		anyZero   bool
	}
	byID := make(map[string]*agg)
	order := make([]string, 0)
	for _, m := range models {
		if m.ProviderID == "" {
			continue
		}
		a, ok := byID[m.ProviderID]
		if !ok {
			a = &agg{name: m.ProviderName}
			byID[m.ProviderID] = a
			order = append(order, m.ProviderID)
		}
		if m.Pricing == nil {
			continue
		}
		if m.Pricing.Prompt == 0 && m.Pricing.Completion == 0 {
			a.anyZero = true
		} else {
			a.anyPriced = true
		}
	}
	sort.Strings(order)

	statuses := make([]ProviderStatus, 0, len(order))
	for _, id := range order {
		a := byID[id]
		st := ProviderStatus{Name: a.name, Configured: true, Ranked: true}
		if !a.anyPriced {
			st.Ranked = false
			if a.anyZero {
				st.ExcludedReason = flatRateReason
			} else {
				st.ExcludedReason = noPricingReason
			}
		}
		statuses = append(statuses, st)
	}
	return statuses
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
//   - quota holds the resolved "quota"-currency ranking denominator: the
//     saturating affordability value for a quota-table-tabulated candidate,
//     or the price/P_min bridge for an untabulated one. It is 0 and unread
//     whenever the role's effective currency is "usd" (design Decision 1
//     parity table).
//   - hasQuota is true only when quota was resolved via the tabulated
//     formula (as opposed to the untabulated fallback bridge); it drives
//     buildReason's staleness surfacing (plan-quota-currency spec,
//     Requirement: Staleness Surfacing).
//   - reqPer5H caches the price-derived requests/5h value used to compute
//     quota, so buildReason never re-derives it at a second call site — the
//     displayed number is guaranteed to equal the ranked one (design: plans
//     schema + derivation).
type scoredModel struct {
	model    EnrichedModel
	qraw     float64
	qnorm    float64
	price    float64
	quota    float64
	hasQuota bool
	reqPer5H float64
}

// RecommendConfig computes the best model for each role in the active
// Profile based on available models and their benchmarks/pricing. It
// returns the list of provider statuses and recommendations sorted by
// criticidad.
func RecommendConfig(ctx context.Context, client *Client, prof *profile.Profile, roleFilter []string) ([]ProviderStatus, []Recommendation, error) {
	providers := []ProviderStatus{
		{Name: "OpenCode Go", Configured: client.OCAPIKey != "", Ranked: true},
		{Name: "OpenRouter", Configured: client.ORAPIKey != "", Ranked: true},
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

	providers = append(providers, discoveredProviderStatuses(models)...)

	roles := filterRoles(prof.Roles, roleFilter)
	if len(roles) == 0 {
		return providers, nil, fmt.Errorf("no matching agent roles for filter: %v", roleFilter)
	}

	// The embedded plan quota table is loaded only when at least one role's
	// effective currency resolves to "quota" — zero I/O otherwise (design
	// Decision 1 parity table; task 3.8).
	var plansTable *plans.Table
	for _, role := range roles {
		if effectiveCurrency(role) == profile.CurrencyQuota {
			pt, err := plans.OpenCodeZenGo()
			if err != nil {
				return providers, nil, fmt.Errorf("loading opencode plan quota table: %w", err)
			}
			plansTable = pt
			break
		}
	}

	// Track which models are already assigned, and which provider family
	// was assigned to each role (for exclude_family_of).
	assignmentCount := make(map[string]int)
	assignedFamily := make(map[string]string)

	// Score models for each role and pick the best.
	recs := make([]Recommendation, 0, len(roles))
	for _, role := range roles {
		best := findBestModel(models, role, assignmentCount, assignedFamily, plansTable)
		if best == nil {
			recs = append(recs, Recommendation{
				Agent:      role.ID,
				Criticidad: role.Criticidad,
				Reason:     noModelReason(models, role, assignmentCount, assignedFamily, plansTable),
			})
			continue
		}

		assignmentCount[best.model.OCID]++
		assignedFamily[role.ID] = familyOf(best.model)

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
			Reason:       buildReason(role, best, plansTable),
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
func findBestModel(models []EnrichedModel, role profile.Role, used map[string]int, assignedFamily map[string]string, plansTable *plans.Table) *scoredModel {
	// First pass: try with max-2 constraint.
	candidates := collectCandidates(models, role, used, 2, assignedFamily, plansTable)
	if len(candidates) == 0 {
		// Second pass: relax the assignment cap to max-3 if no candidates found.
		candidates = collectCandidates(models, role, used, 3, assignedFamily, plansTable)
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

	// The comparator is selected per the role's effective objective
	// (weighted-scoring spec, Requirement: Objective-Selected Comparator;
	// design Decision 4). The denominator each comparator divides by is
	// selected per the role's effective currency (weighted-scoring spec,
	// Requirement: Currency-Selected Denominator) — "usd" resolves to
	// price, unchanged from the pre-PR2a behavior (design Decision 1).
	curr := effectiveCurrency(role)
	switch objective(role) {
	case profile.ObjectiveQuality:
		sortByQuality(candidates, curr)
	case profile.ObjectiveBudget:
		// "budget" reuses the "value" comparator over the set already
		// restricted to max_input_price by collectCandidates' existing
		// hard-constraint pre-filter — no new filter code (design
		// Decision 4; role-profiles spec, Requirement: Budget Objective
		// Requires an Effective Ceiling guarantees this ceiling always
		// exists whenever the effective objective is "budget").
		sortByValue(candidates, curr)
	default: // profile.ObjectiveValue, and the fallback for a nil Selection
		// (a Role built directly, e.g. in tests, without going through
		// profile.Load's merge pass).
		sortByValue(candidates, curr)
	}

	return &candidates[0]
}

// objective returns the role's effective ranking objective, defaulting to
// "value" when Selection is unset. This keeps findBestModel safe for Role
// values built directly (not run through profile.Load's merge pass) and
// matches design Decision 1: an absent selection block always resolves to
// the value objective.
func objective(role profile.Role) string {
	if role.Selection == nil || role.Selection.Objective == "" {
		return profile.ObjectiveValue
	}
	return role.Selection.Objective
}

// effectiveCurrency returns the role's effective ranking currency,
// defaulting to "usd" when Selection is unset. This keeps the ranking
// helpers safe for a Role built directly (not run through profile.Load's
// merge pass) and matches design Decision 1: an absent selection block
// always resolves to the usd currency.
func effectiveCurrency(role profile.Role) string {
	if role.Selection == nil || role.Selection.Currency == "" {
		return profile.CurrencyUSD
	}
	return role.Selection.Currency
}

// genericNoModelReason is the pre-existing "no model available" reason,
// used whenever provider scope is not (at least partly) the cause of an
// empty candidate set.
const genericNoModelReason = "No model with benchmarks available for this role"

// scopeNoModelReason is surfaced instead of genericNoModelReason when the
// role's provider scope excludes every candidate that would otherwise have
// been eligible (design D11; weighted-scoring spec, Requirement: Empty
// Candidate Set Fallback, scenario "No configured provider satisfies the
// role's scope") — a filter without its own reason would otherwise look
// like a silent empty result.
const scopeNoModelReason = "No model available within your configured provider scope for this role"

// noModelReason distinguishes why findBestModel returned nil for a role.
// When the role has a non-empty provider scope and relaxing only that
// constraint (at the fully-relaxed max-3 assignment cap) would have
// produced at least one candidate, provider scope is (at least partly) the
// cause, and scopeNoModelReason is returned; otherwise genericNoModelReason
// is unchanged from pre-PR5 behavior.
func noModelReason(models []EnrichedModel, role profile.Role, used map[string]int, assignedFamily map[string]string, plansTable *plans.Table) string {
	scope := effectiveProviders(role)
	if len(scope) == 0 {
		return genericNoModelReason
	}

	unscoped := role
	sel := *role.Selection
	sel.Providers = nil
	unscoped.Selection = &sel

	if len(collectCandidates(models, unscoped, used, 3, assignedFamily, plansTable)) > 0 {
		return scopeNoModelReason
	}
	return genericNoModelReason
}

// effectiveProviders returns the role's effective provider scope: nil/empty
// means "all configured providers" (role-profiles spec, Requirement:
// Provider Scope Default Is All Configured Providers). Mirrors objective/
// effectiveCurrency's nil-safety for a Role built directly (not run through
// profile.Load's merge pass).
func effectiveProviders(role profile.Role) []string {
	if role.Selection == nil {
		return nil
	}
	return role.Selection.Providers
}

// implicitProviderID is the provider identity assumed for a candidate never
// touched by local CLI discovery: HTTP-sourced OC Go/Zen models keep
// ProviderID empty (Phase 4's byte-identical-output guarantee), but
// mergeDiscovered's own dedupe key already treats such models as belonging
// to "opencode" (api.go's mergeKey("opencode", ...) convention) — the
// provider-scope pre-filter reuses that same implicit identity.
const implicitProviderID = "opencode"

// matchesProviderScope reports whether m's provider identity is included in
// scope. An empty/nil scope means "all configured providers" — never
// filtered. Matching is case-insensitive against ProviderID (design D9),
// and is never validated against live host state.
func matchesProviderScope(m EnrichedModel, scope []string) bool {
	if len(scope) == 0 {
		return true
	}
	id := m.ProviderID
	if id == "" {
		id = implicitProviderID
	}
	for _, s := range scope {
		if strings.EqualFold(strings.TrimSpace(s), id) {
			return true
		}
	}
	return false
}

// slugOf returns the bare model slug for a candidate, stripping the
// "providerID/" CLI-discovery namespace prefix that mergeDiscovered adds to
// OCID when ProviderID != "opencode" (design D5). Falls back to OCID when
// ModelSlug is unset (OC Go/Zen-sourced models untouched by discovery).
func slugOf(m EnrichedModel) string {
	if m.ModelSlug != "" {
		return m.ModelSlug
	}
	return m.OCID
}

// familyOf returns the model-vendor family identity used by
// exclude_family_of. A genuinely distinct CLI-discovered provider
// (ProviderID set and not the "opencode" merge-normalization target) uses
// that ProviderID directly; otherwise family is derived from the bare model
// slug exactly as before discovery existed — this is the fix namespaced CLI
// OCID values (e.g. "openai/gpt-5.6") need: deriveProvider on the raw,
// namespaced OCID would mis-derive the family.
func familyOf(m EnrichedModel) string {
	if m.ProviderID != "" && m.ProviderID != "opencode" {
		return m.ProviderID
	}
	return deriveProvider(slugOf(m))
}

// denominatorOf returns the ranking denominator for a candidate under the
// given effective currency: "usd" resolves to price (unchanged from
// pre-PR2a behavior); "quota" resolves to the pre-resolved quota-currency
// denominator (weighted-scoring spec, Requirement: Currency-Selected
// Denominator).
func denominatorOf(c scoredModel, currency string) float64 {
	if currency == profile.CurrencyQuota {
		return c.quota
	}
	return c.price
}

// sortByValue is the original ranking closure (design Decision 1: unedited
// under "usd", moved verbatim into a shared helper): Qraw/denominator
// descending, tiebreak Qraw descending. It backs both the "value" objective
// and, unmodified, the "budget" objective (design Decision 4). Under "usd"
// currency, denominatorOf resolves to price, so the ranking output is
// byte-identical to the pre-PR2a closure.
func sortByValue(candidates []scoredModel, currency string) {
	sort.Slice(candidates, func(i, j int) bool {
		ri, rj := qualityPriceRatio(candidates[i], currency), qualityPriceRatio(candidates[j], currency)
		if ri != rj {
			return ri > rj
		}
		// Tiebreak: higher raw score wins.
		return candidates[i].qraw > candidates[j].qraw
	})
}

// sortByQuality ranks candidates by Qraw descending, tiebreak by the
// currency-selected denominator ascending, tiebreak OCID ascending for a
// total order (weighted-scoring spec, Requirement: Objective-Selected
// Comparator). The OCID tiebreak makes the order deterministic even when
// Qraw AND the denominator both tie — common on single-metric roles, and
// sort.Slice is unstable — so the golden test never becomes flaky (design
// Decision 4).
func sortByQuality(candidates []scoredModel, currency string) {
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].qraw != candidates[j].qraw {
			return candidates[i].qraw > candidates[j].qraw
		}
		di, dj := denominatorOf(candidates[i], currency), denominatorOf(candidates[j], currency)
		if di != dj {
			return di < dj
		}
		return candidates[i].model.OCID < candidates[j].model.OCID
	})
}

// qualityPriceRatio returns Qraw / denominator for a candidate under the
// given effective currency, or 0 when the denominator is not positive.
func qualityPriceRatio(c scoredModel, currency string) float64 {
	d := denominatorOf(c, currency)
	if d <= 0 {
		return 0
	}
	return c.qraw / d
}

// collectCandidates builds scored models for a role that pass the hard
// constraint pre-filter, in this exact order (weighted-scoring spec, "Hard
// Constraint Pre-Filter"):
//  1. skip models with no usable pricing
//  2. skip models already at maxUses
//  3. apply provider scope, min_context, max_input_price, requires,
//     exclude_family_of — provider scope first, the cheapest check, same
//     tier as the others and never relaxed (design D10)
//  4. for weighted roles, skip candidates with a nil value on any
//     positively-weighted metric
//
// When the role's effective currency is "quota", each returned candidate
// also has its quota-currency denominator resolved (plansTable may be nil
// only when the caller never intends to rank under "quota"; a nil table
// under "quota" currency degrades every candidate to the untabulated
// fallback rather than panicking).
func collectCandidates(models []EnrichedModel, role profile.Role, used map[string]int, maxUses int, assignedFamily map[string]string, plansTable *plans.Table) []scoredModel {
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

		// (3) Hard constraints — never relaxed. Provider scope first: the
		// cheapest check (design D10).
		if !matchesProviderScope(m, effectiveProviders(role)) {
			continue
		}
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
			if fam, ok := assignedFamily[role.ExcludeFamilyOf]; ok && strings.EqualFold(familyOf(m), fam) {
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

	if effectiveCurrency(role) == profile.CurrencyQuota {
		resolveQuotaDenominators(candidates, role, plansTable)
	}

	return candidates
}

// priceOf projects an EnrichedModel's live pricing into a plans.Price value
// (design: internal/profile/plans never imports internal/api; Price is the
// seam). Nil-safe: returns the zero Price when m.Pricing is nil, though
// callers only invoke this after collectCandidates' own nil/zero-price
// gate, so that path is defensive rather than load-bearing.
func priceOf(m EnrichedModel) plans.Price {
	if m.Pricing == nil {
		return plans.Price{}
	}
	p := plans.Price{
		InputPerM:  m.Pricing.Prompt * 1_000_000,
		OutputPerM: m.Pricing.Completion * 1_000_000,
	}
	if m.Pricing.InputCacheRead != nil {
		cacheReadPerM := *m.Pricing.InputCacheRead * 1_000_000
		p.CacheReadPerM = &cacheReadPerM
	}
	return p
}

// resolveQuotaDenominators fills each candidate's quota-currency ranking
// denominator in place: a candidate present in plansTable uses the
// saturating affordability formula
// H_sat / min(requests_per_5h / frequency_weight(role), H_sat), where
// requests_per_5h is derived at request time from the candidate's live
// price via plansTable.Requests (design Decision 2; plan-quota-currency
// spec, Requirement: Frequency Weighting); a candidate absent from the
// table, or whose live price does not resolve a positive derived cost
// (or when plansTable is nil), uses the price/P_min bridge over this
// role's constraint-filtered candidate set — P_min is the minimum price
// across candidates, computed once per call (plan-quota-currency spec,
// Requirement: Fallback for Untabulated Models).
func resolveQuotaDenominators(candidates []scoredModel, role profile.Role, plansTable *plans.Table) {
	if len(candidates) == 0 {
		return
	}

	pMin := math.Inf(1)
	for _, c := range candidates {
		if c.price < pMin {
			pMin = c.price
		}
	}

	fw := profile.FrequencyWeight(role.Frequency)
	for i := range candidates {
		if plansTable != nil {
			if rp5h, ok := plansTable.Requests(candidates[i].model.OCID, priceOf(candidates[i].model), plans.Window5H); ok {
				candidates[i].reqPer5H = rp5h
				headroom := rp5h / fw
				if headroom > hSat {
					headroom = hSat
				}
				candidates[i].quota = hSat / headroom
				candidates[i].hasQuota = true
				continue
			}
		}
		// Fallback: bridge onto the same bounded scale as tabulated
		// candidates via price/P_min instead of a raw, unscaled usd price.
		candidates[i].hasQuota = false
		if pMin > 0 {
			candidates[i].quota = candidates[i].price / pMin
		}
	}
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
// plansTable is consulted only when the role's effective currency is
// "quota", to surface the winning candidate's derived requests_per_5h and
// the table's curation date (plan-quota-currency spec, Requirement:
// Staleness Surfacing); it may be nil. The quota figure is always described
// as derived from the model's current live price, never as a measured
// value — only the curated shape/multiplier/tier inputs behind it carry a
// curation date.
func buildReason(role profile.Role, best *scoredModel, plansTable *plans.Table) string {
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

	// subInfo names the subscription/provider the winning model is served
	// through. OC-native (Go/Zen) subscriptions keep their existing labels;
	// a non-OC provider (CLI-discovered, ProviderName set) must show its own
	// name instead of falling through to "(Zen)", which would mislabel it
	// (design's explicit buildReason fix note).
	subInfo := ""
	switch {
	case m.Subscription == "both":
		subInfo = " (Go + Zen)"
	case m.Subscription == "go":
		subInfo = " (Go)"
	case m.Subscription == "zen":
		subInfo = " (Zen)"
	case m.ProviderName != "":
		subInfo = fmt.Sprintf(" (%s)", m.ProviderName)
	default:
		subInfo = " (Zen)"
	}

	// Under "quota" currency, the Reason format is selected by currency
	// first, regardless of objective: the plan-quota-currency spec's
	// Staleness Surfacing requirement applies to "every recommendation
	// whose denominator was resolved via the table and a live price", not
	// only the "value" objective. A tabulated winner surfaces its derived
	// requests_per_5h — explicitly framed as derived from the current live
	// price, plus the table's curation date for the curated shape/tier
	// inputs — never as a measurement of the request count itself. A
	// fallback winner explicitly marks the absence of quota data and omits
	// any curation-date/staleness claim (design Decision 6; plan-quota-
	// currency spec, Requirement: Staleness Surfacing, both scenarios).
	if effectiveCurrency(role) == profile.CurrencyQuota {
		if best.hasQuota && plansTable != nil {
			return fmt.Sprintf("Mejor calidad/cuota: %s, ~%.0f req/5h (derivado del precio actual; supuestos curados %s), $%.3f/M input%s%s — %s", metric, best.reqPer5H, plansTable.CuratedAt, price, ctxInfo, subInfo, role.Description)
		}
		return fmt.Sprintf("Mejor relación calidad/precio (sin datos de cuota): %s, $%.3f/M input%s%s — %s", metric, price, ctxInfo, subInfo, role.Description)
	}

	// Reason text is selected per the role's effective objective (design
	// Decision 6). The "value" arm is the pre-refactor format, unchanged.
	switch objective(role) {
	case profile.ObjectiveQuality:
		return fmt.Sprintf("Máxima calidad disponible: %s, $%.3f/M input%s%s — %s", metric, price, ctxInfo, subInfo, role.Description)
	case profile.ObjectiveBudget:
		ceiling := 0.0
		if role.MaxInputPrice != nil {
			ceiling = *role.MaxInputPrice
		}
		return fmt.Sprintf("Mejor calidad/precio bajo $%.2f/M: %s, $%.3f/M input%s%s — %s", ceiling, metric, price, ctxInfo, subInfo, role.Description)
	default: // profile.ObjectiveValue, and the fallback for a nil Selection
		return fmt.Sprintf("Mejor relación calidad/precio: %s, $%.3f/M input%s%s — %s", metric, price, ctxInfo, subInfo, role.Description)
	}
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
		// Stated once per provider (design D12): a provider excluded from
		// ranking (e.g. a $0-price flat-rate subscription) is never silently
		// omitted.
		if p.ExcludedReason != "" {
			sb.WriteString(fmt.Sprintf("  ⚠️  %s: %s\n", p.Name, p.ExcludedReason))
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
