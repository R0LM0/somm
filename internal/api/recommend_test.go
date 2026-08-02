package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRecommendConfig_MissingOCKey(t *testing.T) {
	client := &Client{OCAPIKey: "", ORAPIKey: ""}
	_, _, err := RecommendConfig(context.Background(), client, nil)
	if err != ErrOCKeyMissing {
		t.Fatalf("expected ErrOCKeyMissing, got %v", err)
	}
}

func TestRecommendConfig_WithMockModels(t *testing.T) {
	goBody := `{"data":[
		{"id":"deepseek-v4-pro","object":"model","created":1,"owned_by":"test"},
		{"id":"deepseek-v4-flash","object":"model","created":2,"owned_by":"test"},
		{"id":"kimi-k2.6","object":"model","created":3,"owned_by":"test"},
		{"id":"gpt-5.5-pro","object":"model","created":4,"owned_by":"test"}
	]}`
	zenBody := `{"data":[
		{"id":"deepseek-v4-pro","object":"model","created":1,"owned_by":"test"},
		{"id":"deepseek-v4-flash","object":"model","created":2,"owned_by":"test"},
		{"id":"kimi-k2.6","object":"model","created":3,"owned_by":"test"},
		{"id":"gpt-5.5-pro","object":"model","created":4,"owned_by":"test"}
	]}`
	orBody := `{"data":[
		{"id":"deepseek/deepseek-v4-pro","name":"DeepSeek V4 Pro","pricing":{"prompt":"0.000000435","completion":"0.000001"},"context_length":1000000,"benchmarks":{"artificial_analysis":{"intelligence_index":44.3,"coding_index":38.2,"agentic_index":41.5}}},
		{"id":"deepseek/deepseek-v4-flash","name":"DeepSeek V4 Flash","pricing":{"prompt":"0.0000001","completion":"0.0000003"},"context_length":1000000,"benchmarks":{"artificial_analysis":{"intelligence_index":39.1,"coding_index":32.0,"agentic_index":35.0}}},
		{"id":"moonshotai/kimi-k2.6","name":"Kimi K2.6","pricing":{"prompt":"0.00000095","completion":"0.000003"},"context_length":256000,"benchmarks":{"artificial_analysis":{"intelligence_index":52.1,"coding_index":58.6,"agentic_index":55.0}}},
		{"id":"openai/gpt-5.5-pro","name":"GPT-5.5 Pro","pricing":{"prompt":"0.00003","completion":"0.00006"},"context_length":1000000,"benchmarks":{"artificial_analysis":{"intelligence_index":72.5,"coding_index":65.0,"agentic_index":60.0}}}
	]}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/zen/go/v1/models":
			w.Write([]byte(goBody))
		case "/zen/v1/models":
			w.Write([]byte(zenBody))
		case "/api/v1/models":
			w.Write([]byte(orBody))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	client := &Client{
		HTTPClient: rewriteTransport(srv),
		OCAPIKey:   "test-key",
		ORAPIKey:   "test-or-key",
	}

	providers, recs, err := RecommendConfig(context.Background(), client, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify providers.
	if len(providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(providers))
	}
	if !providers[0].Configured {
		t.Error("OpenCode Go should be configured")
	}
	if !providers[1].Configured {
		t.Error("OpenRouter should be configured")
	}

	// We have 4 models with 3 uses each = 12 max recommendations.
	// Not all 19 roles can be assigned, but we should get at least 12.
	if len(recs) < 12 {
		t.Errorf("expected at least 12 recommendations, got %d", len(recs))
	}

	// Verify first recommendation is CRÍTICO (sorted by criticidad).
	if len(recs) > 0 && recs[0].Criticidad != "CRÍTICO" {
		t.Errorf("first rec criticidad = %q, want CRÍTICO", recs[0].Criticidad)
	}

	// Count how many have models (some may not when models run out).
	withModel := 0
	for _, r := range recs {
		if r.Model != "" {
			withModel++
		}
	}
	// At least 12 should have models (4 models × 3 uses).
	if withModel < 12 {
		t.Errorf("expected at least 12 recommendations with models, got %d", withModel)
	}

	// Verify CRÍTICO roles get priority (should all have recommendations).
	criticoRecs := 0
	for _, r := range recs {
		if r.Criticidad == "CRÍTICO" {
			criticoRecs++
		}
	}
	// We have 7 CRÍTICO roles, all should be assigned.
	if criticoRecs < 7 {
		t.Errorf("expected at least 7 CRÍTICO recommendations, got %d", criticoRecs)
	}
}

func TestRecommendConfig_FilterRoles(t *testing.T) {
	goBody := `{"data":[{"id":"deepseek-v4-pro","object":"model","created":1,"owned_by":"test"}]}`
	zenBody := `{"data":[{"id":"deepseek-v4-pro","object":"model","created":1,"owned_by":"test"}]}`
	orBody := `{"data":[{"id":"deepseek/deepseek-v4-pro","name":"DeepSeek V4 Pro","pricing":{"prompt":"0.000000435","completion":"0.000001"},"context_length":1000000,"benchmarks":{"artificial_analysis":{"intelligence_index":44.3,"coding_index":38.2,"agentic_index":41.5}}}]}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/zen/go/v1/models":
			w.Write([]byte(goBody))
		case "/zen/v1/models":
			w.Write([]byte(zenBody))
		case "/api/v1/models":
			w.Write([]byte(orBody))
		}
	}))
	defer srv.Close()

	client := &Client{
		HTTPClient: rewriteTransport(srv),
		OCAPIKey:   "test-key",
		ORAPIKey:   "test-or-key",
	}

	_, recs, err := RecommendConfig(context.Background(), client, []string{"sdd-apply", "sdd-verify"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(recs) != 2 {
		t.Fatalf("expected 2 recommendations, got %d", len(recs))
	}

	agents := make(map[string]bool)
	for _, r := range recs {
		agents[r.Agent] = true
	}
	if !agents["sdd-apply"] || !agents["sdd-verify"] {
		t.Errorf("expected sdd-apply and sdd-verify, got %v", agents)
	}
}

func TestRecommendConfig_InvalidFilter(t *testing.T) {
	goBody := `{"data":[{"id":"deepseek-v4-pro","object":"model","created":1,"owned_by":"test"}]}`
	zenBody := `{"data":[{"id":"deepseek-v4-pro","object":"model","created":1,"owned_by":"test"}]}`
	orBody := `{"data":[]}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/zen/go/v1/models":
			w.Write([]byte(goBody))
		case "/zen/v1/models":
			w.Write([]byte(zenBody))
		case "/api/v1/models":
			w.Write([]byte(orBody))
		}
	}))
	defer srv.Close()

	client := &Client{
		HTTPClient: rewriteTransport(srv),
		OCAPIKey:   "test-key",
		ORAPIKey:   "test-or-key",
	}

	_, _, err := RecommendConfig(context.Background(), client, []string{"nonexistent-role"})
	if err == nil {
		t.Fatal("expected error for invalid filter, got nil")
	}
}

func TestFilterRoles(t *testing.T) {
	all := AllAgentRoles()

	// Empty filter returns all.
	result := filterRoles(all, nil)
	if len(result) != len(all) {
		t.Fatalf("expected %d, got %d", len(all), len(result))
	}

	// Specific filter.
	result = filterRoles(all, []string{"sdd-apply", "sdd-verify"})
	if len(result) != 2 {
		t.Fatalf("expected 2, got %d", len(result))
	}
	if result[0].ID != "sdd-apply" || result[1].ID != "sdd-verify" {
		t.Errorf("unexpected IDs: %v, %v", result[0].ID, result[1].ID)
	}
}

func TestFormatRecommendations(t *testing.T) {
	providers := []ProviderStatus{
		{Name: "OpenCode Go", Configured: true},
		{Name: "OpenRouter", Configured: false},
	}
	intelligence := 44.3
	recs := []Recommendation{
		{
			Agent:       "orchestrator",
			Criticidad:  "CRÍTICO",
			Model:       "DeepSeek V4 Pro",
			Provider:    "DeepSeek",
			PriceInput:  0.435,
			PriceOutput: 1.0,
			Intelligence: &intelligence,
			Reason:      "Mejor relación calidad/precio",
		},
	}

	text := FormatRecommendations(providers, recs)

	if !contains(text, "✅ OpenCode Go") {
		t.Errorf("missing OpenCode Go status in output")
	}
	if !contains(text, "❌ OpenRouter") {
		t.Errorf("missing OpenRouter status in output")
	}
	if !contains(text, "orchestrator → DeepSeek V4 Pro") {
		t.Errorf("missing orchestrator recommendation")
	}
	if !contains(text, "0.435/M input") {
		t.Errorf("missing price in output")
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
