package api

import (
	"math"
	"testing"
)

func ptr[T any](v T) *T {
	return &v
}

func TestMatchOR(t *testing.T) {
	tests := []struct {
		name     string
		ocID     string
		orModels []ORModel
		wantID   string
		wantNil  bool
	}{
		{
			name: "exact match",
			ocID: "deepseek/deepseek-chat",
			orModels: []ORModel{
				{ID: "deepseek/deepseek-chat", Name: "DeepSeek V3"},
				{ID: "openai/gpt-4", Name: "GPT-4"},
			},
			wantID: "deepseek/deepseek-chat",
		},
		{
			name: "case insensitive exact match",
			ocID: "DeepSeek/DeepSeek-Chat",
			orModels: []ORModel{
				{ID: "deepseek/deepseek-chat", Name: "DeepSeek V3"},
			},
			wantID: "deepseek/deepseek-chat",
		},
		{
			name: "provider alias kimi",
			ocID: "kimi-k2",
			orModels: []ORModel{
				{ID: "moonshotai/k2", Name: "Kimi K2"},
				{ID: "moonshotai/kimi-k2", Name: "Kimi K2 Full"},
			},
			wantID: "moonshotai/k2",
		},
		{
			name: "provider alias gpt",
			ocID: "gpt-4",
			orModels: []ORModel{
				{ID: "openai/gpt-4", Name: "GPT-4"},
				{ID: "openai/gpt-3.5-turbo", Name: "GPT-3.5 Turbo"},
			},
			wantID: "openai/gpt-4",
		},
		{
			name: "provider alias qwen prefix",
			ocID: "qwen3",
			orModels: []ORModel{
				{ID: "qwen/3", Name: "Qwen 3"},
				{ID: "qwen/qwen3", Name: "Qwen 3 Full"},
			},
			wantID: "qwen/3",
		},
		{
			name: "alias insertion order takes first matching prefix",
			ocID: "kimi-k2",
			orModels: []ORModel{
				// If aliases were iterated out of order, a hypothetical
				// later alias could win; the first ordered alias (kimi-)
				// must produce the match.
				{ID: "moonshotai/k2", Name: "Kimi K2"},
			},
			wantID: "moonshotai/k2",
		},
		{
			name: "substring single match",
			ocID: "qwen3",
			orModels: []ORModel{
				{ID: "qwen/qwen3", Name: "Qwen 3"},
			},
			wantID: "qwen/qwen3",
		},
		{
			name: "substring multiple matches returns shortest id",
			ocID: "gpt",
			orModels: []ORModel{
				{ID: "openai/gpt-4-turbo-preview", Name: "GPT-4 Turbo"},
				{ID: "openai/gpt-4", Name: "GPT-4"},
				{ID: "openai/gpt-3.5-turbo", Name: "GPT-3.5"},
			},
			wantID: "openai/gpt-4",
		},
		{
			name: "substring precedence lower than alias",
			ocID: "gpt-4",
			orModels: []ORModel{
				{ID: "openai/gpt-4", Name: "GPT-4"},
				{ID: "some-other-gpt-4-turbo", Name: "Other GPT-4"},
			},
			wantID: "openai/gpt-4",
		},
		{
			name: "not found",
			ocID: "nonexistent",
			orModels: []ORModel{
				{ID: "deepseek/deepseek-chat", Name: "DeepSeek V3"},
			},
			wantNil: true,
		},
		{
			name:     "empty or models returns nil",
			ocID:     "deepseek/deepseek-chat",
			orModels: []ORModel{},
			wantNil:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchOR(tt.ocID, tt.orModels)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("expected nil, got %q", got.ID)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected match %q, got nil", tt.wantID)
			}
			if got.ID != tt.wantID {
				t.Errorf("MatchOR(%q) = %q, want %q", tt.ocID, got.ID, tt.wantID)
			}
		})
	}
}

func TestBuildOpenRouterHeaders(t *testing.T) {
	t.Run("empty key omits authorization", func(t *testing.T) {
		h := BuildOpenRouterHeaders("")
		if h.Get("Authorization") != "" {
			t.Errorf("expected no Authorization header, got %q", h.Get("Authorization"))
		}
	})

	t.Run("token becomes bearer", func(t *testing.T) {
		h := BuildOpenRouterHeaders("test-token")
		want := "Bearer test-token"
		if got := h.Get("Authorization"); got != want {
			t.Errorf("Authorization = %q, want %q", got, want)
		}
	})

	t.Run("returns mutable header", func(t *testing.T) {
		h := BuildOpenRouterHeaders("token")
		if h == nil {
			t.Fatal("expected non-nil header map")
		}
		h.Set("X-Custom", "value")
		if h.Get("X-Custom") != "value" {
			t.Error("expected header map to be mutable")
		}
	})
}

func TestEnrichWithOpenRouter(t *testing.T) {
	tests := []struct {
		name       string
		models     []EnrichedModel
		orModels   []ORModel
		assertions func(t *testing.T, got []EnrichedModel)
	}{
		{
			name: "exact match populates all fields",
			models: []EnrichedModel{
				{OCID: "openai/gpt-4", OCName: "GPT-4", OCProvider: "OpenAI", Subscription: "go"},
			},
			orModels: []ORModel{
				{
					ID:            "openai/gpt-4",
					Name:          "GPT-4",
					Pricing:       &Pricing{Prompt: "0.00001", Completion: "0.00003"},
					ContextLength: ptr(int64(8192)),
					Benchmarks: &Benchmarks{ArtificialAnalysis: &ArtificialAnalysis{
						IntelligenceIndex: ptr(75.5),
						CodingIndex:       ptr(72.0),
						AgenticIndex:      ptr(70.0),
					}},
					Reasoning: &Reasoning{
						SupportedEfforts: []string{"low", "medium", "high"},
						DefaultEffort:    ptr("medium"),
						Mandatory:        ptr(true),
						DefaultEnabled:   ptr(false),
					},
				},
			},
			assertions: func(t *testing.T, got []EnrichedModel) {
				m := got[0]
				if m.ORID == nil || *m.ORID != "openai/gpt-4" {
					t.Errorf("ORID = %v, want openai/gpt-4", m.ORID)
				}
				if m.ORName == nil || *m.ORName != "GPT-4" {
					t.Errorf("ORName = %v, want GPT-4", m.ORName)
				}
				if m.Pricing == nil || m.Pricing.Prompt != 0.00001 || m.Pricing.Completion != 0.00003 {
					t.Errorf("Pricing = %+v, want prompt 0.00001 completion 0.00003", m.Pricing)
				}
				if m.ContextLength == nil || *m.ContextLength != 8192 {
					t.Errorf("ContextLength = %v, want 8192", m.ContextLength)
				}
				if m.Benchmarks.Intelligence == nil || *m.Benchmarks.Intelligence != 75.5 {
					t.Errorf("Intelligence = %v, want 75.5", m.Benchmarks.Intelligence)
				}
				if m.Benchmarks.Coding == nil || *m.Benchmarks.Coding != 72.0 {
					t.Errorf("Coding = %v, want 72.0", m.Benchmarks.Coding)
				}
				if m.Benchmarks.Agentic == nil || *m.Benchmarks.Agentic != 70.0 {
					t.Errorf("Agentic = %v, want 70.0", m.Benchmarks.Agentic)
				}
				if m.Reasoning == nil {
					t.Fatal("Reasoning = nil, want non-nil")
				}
				if len(m.Reasoning.SupportedEfforts) != 3 || m.Reasoning.SupportedEfforts[0] != "low" {
					t.Errorf("SupportedEfforts = %v, want [low medium high]", m.Reasoning.SupportedEfforts)
				}
				if m.Reasoning.DefaultEffort == nil || *m.Reasoning.DefaultEffort != "medium" {
					t.Errorf("DefaultEffort = %v, want medium", m.Reasoning.DefaultEffort)
				}
				if !m.Reasoning.Mandatory {
					t.Errorf("Mandatory = %v, want true", m.Reasoning.Mandatory)
				}
				if m.Reasoning.DefaultEnabled {
					t.Errorf("DefaultEnabled = %v, want false", m.Reasoning.DefaultEnabled)
				}
			},
		},
		{
			name: "alias match populates fields",
			models: []EnrichedModel{
				{OCID: "kimi-k2", OCName: "Kimi K2", OCProvider: "Kimi", Subscription: "zen"},
			},
			orModels: []ORModel{
				{
					ID:            "moonshotai/k2",
					Name:          "Kimi K2",
					Pricing:       &Pricing{Prompt: "0.000001", Completion: "0.000002"},
					ContextLength: ptr(int64(256000)),
				},
			},
			assertions: func(t *testing.T, got []EnrichedModel) {
				m := got[0]
				if m.ORID == nil || *m.ORID != "moonshotai/k2" {
					t.Errorf("ORID = %v, want moonshotai/k2", m.ORID)
				}
				if m.Pricing == nil || m.Pricing.Prompt != 0.000001 || m.Pricing.Completion != 0.000002 {
					t.Errorf("Pricing = %+v, want prompt 0.000001 completion 0.000002", m.Pricing)
				}
				if m.ContextLength == nil || *m.ContextLength != 256000 {
					t.Errorf("ContextLength = %v, want 256000", m.ContextLength)
				}
			},
		},
		{
			name: "no match leaves zero values",
			models: []EnrichedModel{
				{OCID: "unknown/model", OCName: "Unknown", OCProvider: "Unknown", Subscription: "go"},
			},
			orModels: []ORModel{
				{ID: "openai/gpt-4", Name: "GPT-4"},
			},
			assertions: func(t *testing.T, got []EnrichedModel) {
				m := got[0]
				if m.ORID != nil {
					t.Errorf("ORID = %v, want nil", m.ORID)
				}
				if m.ORName != nil {
					t.Errorf("ORName = %v, want nil", m.ORName)
				}
				if m.Pricing != nil {
					t.Errorf("Pricing = %+v, want nil", m.Pricing)
				}
				if m.ContextLength != nil {
					t.Errorf("ContextLength = %v, want nil", m.ContextLength)
				}
				if m.Reasoning != nil {
					t.Errorf("Reasoning = %+v, want nil", m.Reasoning)
				}
			},
		},
		{
			name: "multiple models mixed match and no match",
			models: []EnrichedModel{
				{OCID: "openai/gpt-4", OCName: "GPT-4", OCProvider: "OpenAI", Subscription: "go"},
				{OCID: "unknown/model", OCName: "Unknown", OCProvider: "Unknown", Subscription: "zen"},
				{OCID: "deepseek/deepseek-chat", OCName: "DeepSeek Chat", OCProvider: "DeepSeek", Subscription: "both"},
			},
			orModels: []ORModel{
				{
					ID:      "openai/gpt-4",
					Name:    "GPT-4",
					Pricing: &Pricing{Prompt: "0.00001", Completion: "0.00003"},
				},
				{
					ID:            "deepseek/deepseek-chat",
					Name:          "DeepSeek V3",
					ContextLength: ptr(int64(64000)),
				},
			},
			assertions: func(t *testing.T, got []EnrichedModel) {
				if got[0].Pricing == nil {
					t.Errorf("model 0 Pricing = nil, want non-nil")
				}
				if got[1].Pricing != nil || got[1].ORID != nil {
					t.Errorf("model 1 should remain unenriched")
				}
				if got[2].ContextLength == nil || *got[2].ContextLength != 64000 {
					t.Errorf("model 2 ContextLength = %v, want 64000", got[2].ContextLength)
				}
			},
		},
		{
			name: "context length falls back to top provider",
			models: []EnrichedModel{
				{OCID: "openai/gpt-4", OCName: "GPT-4", OCProvider: "OpenAI", Subscription: "go"},
			},
			orModels: []ORModel{
				{
					ID:   "openai/gpt-4",
					Name: "GPT-4",
					TopProvider: &TopProvider{
						ContextLength: ptr(int64(128000)),
					},
				},
			},
			assertions: func(t *testing.T, got []EnrichedModel) {
				if got[0].ContextLength == nil || *got[0].ContextLength != 128000 {
					t.Errorf("ContextLength = %v, want 128000", got[0].ContextLength)
				}
			},
		},
		{
			name: "benchmark fields stay nil when artificial analysis absent",
			models: []EnrichedModel{
				{OCID: "openai/gpt-4", OCName: "GPT-4", OCProvider: "OpenAI", Subscription: "go"},
			},
			orModels: []ORModel{
				{
					ID:   "openai/gpt-4",
					Name: "GPT-4",
					Benchmarks: &Benchmarks{
						ArtificialAnalysis: &ArtificialAnalysis{
							IntelligenceIndex: ptr(80.0),
						},
					},
				},
			},
			assertions: func(t *testing.T, got []EnrichedModel) {
				if got[0].Benchmarks.Intelligence == nil || *got[0].Benchmarks.Intelligence != 80.0 {
					t.Errorf("Intelligence = %v, want 80.0", got[0].Benchmarks.Intelligence)
				}
				if got[0].Benchmarks.Coding != nil || got[0].Benchmarks.Agentic != nil {
					t.Errorf("Coding/Agentic should be nil, got coding=%v agentic=%v", got[0].Benchmarks.Coding, got[0].Benchmarks.Agentic)
				}
			},
		},
		{
			name: "reasoning defaults when only supported efforts present",
			models: []EnrichedModel{
				{OCID: "openai/gpt-4", OCName: "GPT-4", OCProvider: "OpenAI", Subscription: "go"},
			},
			orModels: []ORModel{
				{
					ID:   "openai/gpt-4",
					Name: "GPT-4",
					Reasoning: &Reasoning{
						SupportedEfforts: []string{"low", "high"},
					},
				},
			},
			assertions: func(t *testing.T, got []EnrichedModel) {
				r := got[0].Reasoning
				if r == nil {
					t.Fatal("Reasoning = nil")
				}
				if len(r.SupportedEfforts) != 2 {
					t.Errorf("SupportedEfforts = %v, want 2", r.SupportedEfforts)
				}
				if r.DefaultEffort != nil {
					t.Errorf("DefaultEffort = %v, want nil", r.DefaultEffort)
				}
				if r.Mandatory {
					t.Errorf("Mandatory = true, want false")
				}
				if !r.DefaultEnabled {
					t.Errorf("DefaultEnabled = false, want true (because mandatory is false)")
				}
			},
		},
		{
			name: "pricing strings parse invalid and empty as zero",
			models: []EnrichedModel{
				{OCID: "openai/gpt-4", OCName: "GPT-4", OCProvider: "OpenAI", Subscription: "go"},
			},
			orModels: []ORModel{
				{
					ID:   "openai/gpt-4",
					Name: "GPT-4",
					Pricing: &Pricing{
						Prompt:     "not-a-number",
						Completion: "",
					},
				},
			},
			assertions: func(t *testing.T, got []EnrichedModel) {
				if got[0].Pricing == nil {
					t.Fatal("Pricing = nil")
				}
				if got[0].Pricing.Prompt != 0 || got[0].Pricing.Completion != 0 {
					t.Errorf("Pricing = %+v, want zero values", got[0].Pricing)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enrichWithOpenRouter(tt.models, tt.orModels)
			tt.assertions(t, tt.models)
		})
	}
}

func TestParseMoney(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want float64
	}{
		{"parses decimal string", "0.000001", 0.000001},
		{"parses larger value", "0.5", 0.5},
		{"empty string is zero", "", 0},
		{"invalid string is zero", "free", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseMoney(tt.in)
			if math.Abs(got-tt.want) > 1e-12 {
				t.Errorf("ParseMoney(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
