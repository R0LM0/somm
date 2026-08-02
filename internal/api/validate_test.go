package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidateConfig_MissingOCKey(t *testing.T) {
	client := &Client{OCAPIKey: "", ORAPIKey: ""}
	_, err := ValidateConfig(context.Background(), client)
	if err == nil {
		t.Fatal("expected error for missing OC key, got nil")
	}
}

func TestValidateConfig_WithMockModels(t *testing.T) {
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

	result, err := ValidateConfig(context.Background(), client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify providers.
	if len(result.Providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(result.Providers))
	}
	if !result.Providers[0].Configured {
		t.Error("OpenCode Go should be configured")
	}
	if !result.Providers[1].Configured {
		t.Error("OpenRouter should be configured")
	}

	// Verify agent checks.
	allRoles := AllAgentRoles()
	if len(result.AgentChecks) != len(allRoles) {
		t.Fatalf("expected %d agent checks, got %d", len(allRoles), len(result.AgentChecks))
	}

	// Verify score is in range.
	if result.Score < 0 || result.Score > 10 {
		t.Errorf("score = %d, want 0-10", result.Score)
	}
}

func TestValidateConfig_NoOpenRouter(t *testing.T) {
	goBody := `{"data":[{"id":"deepseek-v4-pro","object":"model","created":1,"owned_by":"test"}]}`
	zenBody := `{"data":[{"id":"deepseek-v4-pro","object":"model","created":1,"owned_by":"test"}]}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/zen/go/v1/models":
			w.Write([]byte(goBody))
		case "/zen/v1/models":
			w.Write([]byte(zenBody))
		case "/api/v1/models":
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("error"))
		}
	}))
	defer srv.Close()

	client := &Client{
		HTTPClient: rewriteTransport(srv),
		OCAPIKey:   "test-key",
		ORAPIKey:   "",
	}

	result, err := ValidateConfig(context.Background(), client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have improvement for missing OpenRouter.
	found := false
	for _, imp := range result.Improvements {
		if contains(imp, "OPENROUTER_API_KEY") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected improvement for missing OPENROUTER_API_KEY, got: %v", result.Improvements)
	}
}

func TestFormatValidation(t *testing.T) {
	result := &ValidationResult{
		Providers: []ProviderStatusDetail{
			{Name: "OpenCode Go", Configured: true, Models: 45},
			{Name: "OpenRouter", Configured: false, Models: 0},
		},
		AgentChecks: []AgentCheck{
			{
				Agent:        "orchestrator",
				CurrentModel: "DeepSeek V4 Pro",
				Status:       "optimal",
				Suggestion:   "Mejor relación calidad/precio",
			},
			{
				Agent:        "sdd-apply",
				CurrentModel: "",
				Status:       "error",
				Suggestion:   "No hay modelo disponible",
			},
		},
		Score:        7,
		Improvements: []string{"Agregar OPENROUTER_API_KEY"},
	}

	text := FormatValidation(result)

	if !contains(text, "✅ OpenCode Go: Configurado (45 modelos)") {
		t.Errorf("missing OpenCode Go status in output")
	}
	if !contains(text, "❌ OpenRouter: No detectado") {
		t.Errorf("missing OpenRouter status in output")
	}
	if !contains(text, "orchestrator (DeepSeek V4 Pro):") {
		t.Errorf("missing orchestrator in output")
	}
	if !contains(text, "Score de configuración: 7/10") {
		t.Errorf("missing score in output")
	}
	if !contains(text, "1. Agregar OPENROUTER_API_KEY") {
		t.Errorf("missing improvements in output")
	}
}
