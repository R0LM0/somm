package api

import "net/http"

// OCModel is an OpenCode model entry as returned by the OpenCode API.
type OCModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// Pricing describes per-token costs from OpenRouter as decimal strings.
type Pricing struct {
	Prompt         string  `json:"prompt"`
	Completion     string  `json:"completion"`
	InputCacheRead *string `json:"input_cache_read,omitempty"`
}

// Architecture describes model architecture metadata from OpenRouter.
type Architecture struct {
	Modality     *string `json:"modality,omitempty"`
	Tokenizer    *string `json:"tokenizer,omitempty"`
	InstructType *string `json:"instruct_type,omitempty"`
}

// TopProvider describes the hosting provider's limits from OpenRouter.
type TopProvider struct {
	ContextLength       *int64 `json:"context_length,omitempty"`
	MaxCompletionTokens *int64 `json:"max_completion_tokens,omitempty"`
	IsModerated         *bool  `json:"is_moderated,omitempty"`
}

// ArtificialAnalysis holds benchmark scores from OpenRouter.
type ArtificialAnalysis struct {
	IntelligenceIndex *float64 `json:"intelligence_index,omitempty"`
	CodingIndex       *float64 `json:"coding_index,omitempty"`
	AgenticIndex      *float64 `json:"agentic_index,omitempty"`
}

// Benchmarks groups the benchmark sources available on OpenRouter.
type Benchmarks struct {
	ArtificialAnalysis *ArtificialAnalysis `json:"artificial_analysis,omitempty"`
}

// Reasoning describes reasoning-effort support from OpenRouter.
type Reasoning struct {
	SupportedEfforts []string `json:"supported_efforts,omitempty"`
	DefaultEffort    *string  `json:"default_effort,omitempty"`
	DefaultEnabled   *bool    `json:"default_enabled,omitempty"`
	Mandatory        *bool    `json:"mandatory,omitempty"`
}

// ORModel is an OpenRouter model entry as returned by the OpenRouter API.
type ORModel struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	Description   *string       `json:"description,omitempty"`
	Pricing       *Pricing      `json:"pricing,omitempty"`
	ContextLength *int64        `json:"context_length,omitempty"`
	Architecture  *Architecture `json:"architecture,omitempty"`
	TopProvider   *TopProvider  `json:"top_provider,omitempty"`
	Benchmarks    *Benchmarks   `json:"benchmarks,omitempty"`
	Reasoning     *Reasoning    `json:"reasoning,omitempty"`
}

// Money is a parsed pricing pair used in the enriched model view.
type Money struct {
	Prompt         float64  `json:"prompt"`
	Completion     float64  `json:"completion"`
	InputCacheRead *float64 `json:"input_cache_read,omitempty"`
}

// ModelBenchmarks is the internal benchmark view for an enriched model.
type ModelBenchmarks struct {
	Intelligence *float64 `json:"intelligence"`
	Coding       *float64 `json:"coding"`
	Agentic      *float64 `json:"agentic"`
}

// ModelReasoning is the internal reasoning view for an enriched model.
type ModelReasoning struct {
	SupportedEfforts []string `json:"supportedEfforts"`
	DefaultEffort    *string  `json:"defaultEffort"`
	Mandatory        bool     `json:"mandatory"`
	DefaultEnabled   bool     `json:"defaultEnabled"`
}

// EnrichedModel combines an OpenCode model with optional OpenRouter data.
type EnrichedModel struct {
	OCID          string          `json:"ocId"`
	OCName        string          `json:"ocName"`
	OCProvider    string          `json:"ocProvider"`
	ORID          *string         `json:"orId"`
	ORName        *string         `json:"orName"`
	Pricing       *Money          `json:"pricing"`
	ContextLength *int64          `json:"contextLength"`
	Benchmarks    ModelBenchmarks `json:"benchmarks"`
	Subscription  string          `json:"subscription"`
	Reasoning     *ModelReasoning `json:"reasoning"`

	// ProviderID, ProviderName, ModelSlug, and PriceSource are populated only
	// for models touched by local `opencode` CLI discovery (design D5/D6/D7).
	// All four are omitempty so output stays byte-identical to today's shape
	// when discovery is absent (design Migration/Rollout).
	ProviderID   string `json:"providerId,omitempty"`
	ProviderName string `json:"providerName,omitempty"`
	ModelSlug    string `json:"modelSlug,omitempty"`
	// PriceSource is "opencode-cli" once a CLI-sourced price has been applied;
	// enrichWithOpenRouter MUST NOT overwrite Pricing while this is set
	// (design D6 — the CLI's first-party price wins over the OpenRouter proxy
	// price).
	PriceSource string `json:"priceSource,omitempty"`
}

// Client is the HTTP client for OpenCode and OpenRouter APIs.
// A nil HTTPClient is interpreted as http.DefaultClient at request time.
type Client struct {
	HTTPClient *http.Client
	OCAPIKey   string
	ORAPIKey   string
	// Discoverer reports models served by providers configured on this host
	// (design D1/D2). nil means the default execDiscoverer (production
	// behavior, per Requirement: Discovery Is On By Default); tests inject a
	// fake or a no-op (design Interfaces/Contracts comment).
	Discoverer ProviderDiscoverer
}
