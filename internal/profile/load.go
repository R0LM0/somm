package profile

import (
	"fmt"

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

	return &p, nil
}

// validate enforces the profile schema: every role must declare an id, and
// every weights key must be a recognized metric.
func validate(p *Profile) error {
	for i, r := range p.Roles {
		if r.ID == "" {
			return fmt.Errorf("role at index %d is missing required field %q", i, "id")
		}
		for metric := range r.Weights {
			if !validMetrics[metric] {
				return fmt.Errorf("role %q: unknown metric key %q in weights (valid keys: intelligence, coding, agentic)", r.ID, metric)
			}
		}
	}
	return nil
}

// mergeDefaults inherits defaults.min_context, defaults.max_input_price,
// and defaults.requires into any role that leaves the corresponding field
// nil. weights is never defaulted.
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
	}
}
