package profile

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Load parses and validates a Profile from raw YAML bytes, then merges
// Defaults into any role that leaves the corresponding hard-constraint
// field unset. Load fails loud: malformed YAML, an unknown weights metric
// key, or a role missing id all return an error instead of a silent
// fallback.
func Load(data []byte) (*Profile, error) {
	var p Profile
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parsing profile YAML: %w", err)
	}

	if err := validate(&p); err != nil {
		return nil, err
	}

	mergeDefaults(&p)

	if err := validatePostMerge(&p); err != nil {
		return nil, err
	}

	return &p, nil
}

// validatePostMerge enforces constraints that depend on merged state. A
// role whose effective objective is "budget" must have an effective
// max_input_price ceiling (role-level or inherited from defaults) — it
// MUST NOT silently degrade to "value" ranking for lack of one
// (role-profiles spec, Requirement: Budget Objective Requires an Effective
// Ceiling).
func validatePostMerge(p *Profile) error {
	for _, r := range p.Roles {
		if r.Selection != nil && r.Selection.Objective == ObjectiveBudget && r.MaxInputPrice == nil {
			return fmt.Errorf("role %q: selection.objective %q requires an effective max_input_price ceiling (set it on the role or on defaults)", r.ID, ObjectiveBudget)
		}
	}
	return nil
}

// validate enforces the profile schema: every role must declare an id,
// every weights key must be a recognized metric, and any selection block
// (profile- or role-level) must use recognized objective/currency values.
func validate(p *Profile) error {
	if err := validateSelection(p.Selection, "profile-level selection"); err != nil {
		return err
	}
	for i, r := range p.Roles {
		if r.ID == "" {
			return fmt.Errorf("role at index %d is missing required field %q", i, "id")
		}
		for metric := range r.Weights {
			if !validMetrics[metric] {
				return fmt.Errorf("role %q: unknown metric key %q in weights (valid keys: intelligence, coding, agentic)", r.ID, metric)
			}
		}
		if err := validateSelection(r.Selection, fmt.Sprintf("role %q selection", r.ID)); err != nil {
			return err
		}
		if r.Frequency != "" && !validFrequencies[r.Frequency] {
			return fmt.Errorf("role %q: unknown frequency value %q (valid: high, medium, low)", r.ID, r.Frequency)
		}
	}
	return nil
}

// validateSelection rejects an unrecognized objective or currency value, a
// malformed providers entry, or an explicitly empty providers list in a
// Selection block. A nil block or an unset field is always valid — enum
// checks apply only to values actually present (role-profiles spec,
// Requirement: Selection Block Schema).
func validateSelection(s *Selection, context string) error {
	if s == nil {
		return nil
	}
	if s.Objective != "" && !validObjectives[s.Objective] {
		return fmt.Errorf("%s: unknown objective value %q (valid: value, quality, budget)", context, s.Objective)
	}
	if s.Currency != "" && !validCurrencies[s.Currency] {
		return fmt.Errorf("%s: unknown currency value %q (valid: usd, quota)", context, s.Currency)
	}
	// Providers != nil but len == 0 means the YAML explicitly wrote
	// `providers: []`, distinct from the field being absent entirely
	// (nil). An explicit empty list is a load error (D8) because it is an
	// un-expressible footgun: omitting the field is the correct way to
	// mean "all configured providers".
	if s.Providers != nil && len(s.Providers) == 0 {
		return fmt.Errorf("%s: providers is explicitly empty; omit the field entirely to mean all configured providers", context)
	}
	for i, provider := range s.Providers {
		if strings.TrimSpace(provider) == "" {
			return fmt.Errorf("%s: providers entry %d is empty or whitespace-only", context, i)
		}
	}
	return nil
}

// mergeDefaults inherits defaults.min_context, defaults.max_input_price,
// and defaults.requires into any role that leaves the corresponding field
// nil. weights is never defaulted. It also resolves each role's effective
// Selection.
func mergeDefaults(p *Profile) {
	for i := range p.Roles {
		r := &p.Roles[i]
		if r.MinContext == nil {
			r.MinContext = p.Defaults.MinContext
		}
		if r.MaxInputPrice == nil {
			r.MaxInputPrice = p.Defaults.MaxInputPrice
		}
		if r.Requires == nil {
			r.Requires = p.Defaults.Requires
		}
		resolveSelection(r, p.Selection)
	}
}

// resolveSelection overwrites r.Selection with the fully resolved,
// non-nil effective selection, applying each field independently in
// precedence order role -> profile -> {value, usd} (role-profiles spec,
// Requirement: Per-Role Selection Override). Providers follows the same
// scalar-replace precedence: nil (unset at that level) inherits from the
// next level down; a non-nil (and, post-validation, non-empty) list
// replaces it entirely (role-profiles spec, Requirement: Per-Role
// Selection Override; Requirement: Provider Scope Default Is All
// Configured Providers). It also records whether the effective currency
// came from an explicit role- or profile-level selection.currency, as
// opposed to the {value, usd} fallback.
func resolveSelection(r *Role, profileSel *Selection) {
	objective := ObjectiveValue
	currency := CurrencyUSD
	currencyExplicit := false
	var providers []string

	if profileSel != nil {
		if profileSel.Objective != "" {
			objective = profileSel.Objective
		}
		if profileSel.Currency != "" {
			currency = profileSel.Currency
			currencyExplicit = true
		}
		if profileSel.Providers != nil {
			providers = profileSel.Providers
		}
	}

	if r.Selection != nil {
		if r.Selection.Objective != "" {
			objective = r.Selection.Objective
		}
		if r.Selection.Currency != "" {
			currency = r.Selection.Currency
			currencyExplicit = true
		}
		if r.Selection.Providers != nil {
			providers = r.Selection.Providers
		}
	}

	r.Selection = &Selection{Objective: objective, Currency: currency, Providers: providers}
	r.currencyExplicit = currencyExplicit
}
