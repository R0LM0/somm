package api_test

import (
	"testing"

	"github.com/AlonsoSG0/model-advisor-mcp/internal/api"
)

func TestMatchOR(t *testing.T) {
	tests := []struct {
		name     string
		ocID     string
		orModels []api.ORModel
		wantID   string
		wantNil  bool
	}{
		{
			name: "exact match",
			ocID: "deepseek/deepseek-chat",
			orModels: []api.ORModel{
				{ID: "deepseek/deepseek-chat", Name: "DeepSeek V3"},
				{ID: "openai/gpt-4", Name: "GPT-4"},
			},
			wantID: "deepseek/deepseek-chat",
		},
		{
			name: "case insensitive exact match",
			ocID: "DeepSeek/DeepSeek-Chat",
			orModels: []api.ORModel{
				{ID: "deepseek/deepseek-chat", Name: "DeepSeek V3"},
			},
			wantID: "deepseek/deepseek-chat",
		},
		{
			name: "provider alias kimi",
			ocID: "kimi-k2",
			orModels: []api.ORModel{
				{ID: "moonshotai/k2", Name: "Kimi K2"},
				{ID: "moonshotai/kimi-k2", Name: "Kimi K2 Full"},
			},
			wantID: "moonshotai/k2",
		},
		{
			name: "provider alias gpt",
			ocID: "gpt-4",
			orModels: []api.ORModel{
				{ID: "openai/gpt-4", Name: "GPT-4"},
				{ID: "openai/gpt-3.5-turbo", Name: "GPT-3.5 Turbo"},
			},
			wantID: "openai/gpt-4",
		},
		{
			name: "provider alias qwen prefix",
			ocID: "qwen3",
			orModels: []api.ORModel{
				{ID: "qwen/3", Name: "Qwen 3"},
				{ID: "qwen/qwen3", Name: "Qwen 3 Full"},
			},
			wantID: "qwen/3",
		},
		{
			name: "alias insertion order takes first matching prefix",
			ocID: "kimi-k2",
			orModels: []api.ORModel{
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
			orModels: []api.ORModel{
				{ID: "qwen/qwen3", Name: "Qwen 3"},
			},
			wantID: "qwen/qwen3",
		},
		{
			name: "substring multiple matches returns shortest id",
			ocID: "gpt",
			orModels: []api.ORModel{
				{ID: "openai/gpt-4-turbo-preview", Name: "GPT-4 Turbo"},
				{ID: "openai/gpt-4", Name: "GPT-4"},
				{ID: "openai/gpt-3.5-turbo", Name: "GPT-3.5"},
			},
			wantID: "openai/gpt-4",
		},
		{
			name: "substring precedence lower than alias",
			ocID: "gpt-4",
			orModels: []api.ORModel{
				{ID: "openai/gpt-4", Name: "GPT-4"},
				{ID: "some-other-gpt-4-turbo", Name: "Other GPT-4"},
			},
			wantID: "openai/gpt-4",
		},
		{
			name: "not found",
			ocID: "nonexistent",
			orModels: []api.ORModel{
				{ID: "deepseek/deepseek-chat", Name: "DeepSeek V3"},
			},
			wantNil: true,
		},
		{
			name:     "empty or models returns nil",
			ocID:     "deepseek/deepseek-chat",
			orModels: []api.ORModel{},
			wantNil:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := api.MatchOR(tt.ocID, tt.orModels)
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
		h := api.BuildOpenRouterHeaders("")
		if h.Get("Authorization") != "" {
			t.Errorf("expected no Authorization header, got %q", h.Get("Authorization"))
		}
	})

	t.Run("token becomes bearer", func(t *testing.T) {
		h := api.BuildOpenRouterHeaders("test-token")
		want := "Bearer test-token"
		if got := h.Get("Authorization"); got != want {
			t.Errorf("Authorization = %q, want %q", got, want)
		}
	})

	t.Run("returns mutable header", func(t *testing.T) {
		h := api.BuildOpenRouterHeaders("token")
		if h == nil {
			t.Fatal("expected non-nil header map")
		}
		h.Set("X-Custom", "value")
		if h.Get("X-Custom") != "value" {
			t.Error("expected header map to be mutable")
		}
	})
}
