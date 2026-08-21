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

// Objective and currency values accepted in a Selection block.
const (
	ObjectiveValue   = "value"
	ObjectiveQuality = "quality"
	ObjectiveBudget  = "budget"

	CurrencyUSD   = "usd"
	CurrencyQuota = "quota"
)

// validObjectives is the set of objective values accepted in a Selection
// block.
var validObjectives = map[string]bool{
	ObjectiveValue:   true,
	ObjectiveQuality: true,
	ObjectiveBudget:  true,
}

// validCurrencies is the set of currency values accepted in a Selection
// block.
var validCurrencies = map[string]bool{
	CurrencyUSD:   true,
	CurrencyQuota: true,
}

// Selection controls which comparator ranks candidates (Objective) and
// which denominator the comparator divides by (Currency). Both fields are
// independent and resolve at load time: an absent block, or an absent
// field within a present block, resolves to {value, usd} through profile
// then role-level inheritance (see Requirement: Selection Block Schema,
// Requirement: Per-Role Selection Override).
type Selection struct {
	Objective string `yaml:"objective,omitempty"` // value | quality | budget
	Currency  string `yaml:"currency,omitempty"`  // usd | quota
}

// Profile is the top-level YAML document describing the active set of
// agent roles and their scoring/constraint configuration.
type Profile struct {
	Version   int        `yaml:"version"`
	Defaults  Defaults   `yaml:"defaults"`
	Selection *Selection `yaml:"selection,omitempty"`
	Roles     []Role     `yaml:"roles"`
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
	// Selection is a role-level override of the profile-level Selection
	// block (yaml input). After Load, it is overwritten with the fully
	// resolved, non-nil effective selection for this role (role -> profile
	// -> {value, usd}) — see Requirement: Per-Role Selection Override.
	Selection *Selection `yaml:"selection,omitempty"`
	// currencyExplicit records whether the resolved Currency came from an
	// explicit role- or profile-level selection.currency, as opposed to the
	// {value, usd} fallback. Unexported: not YAML input, set during merge.
	currencyExplicit bool
}
