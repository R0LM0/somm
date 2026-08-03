// Package profile defines the provider-agnostic role profile schema used to
// drive somm's model recommendation engine: role definitions, scoring
// weights, hard constraints, and the resolution order used to select the
// active profile at startup. The embedded gentle-ai preset reproduces
// today's hardcoded role taxonomy as the built-in default.
package profile

// Metric key constants restrict the keys allowed in a Role's Weights map.
// There is intentionally no "speed" metric: the underlying benchmark data
// has no speed dimension.
const (
	MetricIntelligence = "intelligence"
	MetricCoding       = "coding"
	MetricAgentic      = "agentic"
)

// validMetrics is the set of metric keys accepted in a role's Weights map.
var validMetrics = map[string]bool{
	MetricIntelligence: true,
	MetricCoding:       true,
	MetricAgentic:      true,
}

// Profile is the top-level YAML document describing the active set of
// agent roles and their scoring/constraint configuration.
type Profile struct {
	Version  int      `yaml:"version"`
	Defaults Defaults `yaml:"defaults"`
	Roles    []Role   `yaml:"roles"`
}

// Defaults holds hard-constraint values inherited by any role that leaves
// the corresponding field unset. Weights are never defaulted: they are
// always role-specific.
type Defaults struct {
	MinContext    *int64   `yaml:"min_context,omitempty"`
	MaxInputPrice *float64 `yaml:"max_input_price,omitempty"`
	Requires      []string `yaml:"requires,omitempty"`
}

// Role defines a single agent role: identity, scoring weights, and hard
// constraints used to filter and rank candidate models.
type Role struct {
	ID              string             `yaml:"id"`
	Description     string             `yaml:"description,omitempty"`
	Criticidad      string             `yaml:"criticidad,omitempty"`
	Weights         map[string]float64 `yaml:"weights,omitempty"`
	MinContext      *int64             `yaml:"min_context,omitempty"`
	MaxInputPrice   *float64           `yaml:"max_input_price,omitempty"`
	Requires        []string           `yaml:"requires,omitempty"`
	ExcludeFamilyOf string             `yaml:"exclude_family_of,omitempty"`
}
