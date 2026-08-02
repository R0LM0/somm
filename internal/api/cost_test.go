package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDefaultTokenUsage(t *testing.T) {
	tests := []struct {
		agent string
		want  TokenUsage
	}{
		{"orchestrator", TokenUsage{Input: 50_000, Output: 10_000}},
		{"sdd-apply", TokenUsage{Input: 100_000, Output: 50_000}},
		{"sdd-verify", TokenUsage{Input: 80_000, Output: 20_000}},
		{"sdd-explore", TokenUsage{Input: 60_000, Output: 15_000}},
		{"review-risk", TokenUsage{Input: 30_000, Output: 10_000}},
		{"jd-judge-a", TokenUsage{Input: 30_000, Output: 10_000}},
		{"unknown-agent", TokenUsage{Input: 30_000, Output: 10_000}},
	}

	for _, tt := range tests {
		t.Run(tt.agent, func(t *testing.T) {
			got := defaultTokenUsage(tt.agent)
			if got != tt.want {
				t.Errorf("defaultTokenUsage(%q) = %v, want %v", tt.agent, got, tt.want)
			}
		})
	}
}

func TestEstimateCost_WithMockModels(t *testing.T) {
	goBody := `{"data":[
		{"id":"deepseek-v4-pro","object":"model","created":1,"owned_by":"test"},
		{"id":"kimi-k2.6","object":"model","created":3,"owned_by":"test"}
	]}`
	zenBody := `{"data":[
		{"id":"deepseek-v4-pro","object":"model","created":1,"owned_by":"test"},
		{"id":"kimi-k2.6","object":"model","created":3,"owned_by":"test"}
	]}`
	orBody := `{"data":[
		{"id":"deepseek/deepseek-v4-pro","name":"DeepSeek V4 Pro","pricing":{"prompt":"0.000000435","completion":"0.000001"},"context_length":1000000,"benchmarks":{"artificial_analysis":{"intelligence_index":44.3,"coding_index":38.2,"agentic_index":41.5}}},
		{"id":"moonshotai/kimi-k2.6","name":"Kimi K2.6","pricing":{"prompt":"0.00000095","completion":"0.000003"},"context_length":256000,"benchmarks":{"artificial_analysis":{"intelligence_index":52.1,"coding_index":58.6,"agentic_index":55.0}}}
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

	result, err := EstimateCost(context.Background(), client, 8, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.HoursPerDay != 8 {
		t.Errorf("HoursPerDay = %v, want 8", result.HoursPerDay)
	}
	if result.DaysPerMonth != 22 {
		t.Errorf("DaysPerMonth = %v, want 22", result.DaysPerMonth)
	}
	if len(result.Estimates) == 0 {
		t.Fatal("expected at least 1 estimate")
	}

	// Verify sessions per month = 8 * 22 = 176.
	for _, est := range result.Estimates {
		if est.SessionsPerMonth != 176 {
			t.Errorf("%s: SessionsPerMonth = %d, want 176", est.Agent, est.SessionsPerMonth)
		}
	}

	// Total monthly should be positive.
	if result.TotalMonthly <= 0 {
		t.Errorf("TotalMonthly = %v, want > 0", result.TotalMonthly)
	}

	// ByProvider should have at least one entry.
	if len(result.ByProvider) == 0 {
		t.Error("ByProvider should have at least one entry")
	}
}

func TestEstimateCost_FilterRoles(t *testing.T) {
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

	result, err := EstimateCost(context.Background(), client, 8, []string{"orchestrator"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Estimates) != 1 {
		t.Fatalf("expected 1 estimate, got %d", len(result.Estimates))
	}
	if result.Estimates[0].Agent != "orchestrator" {
		t.Errorf("expected orchestrator, got %s", result.Estimates[0].Agent)
	}
}

func TestEstimateCost_DefaultHoursPerDay(t *testing.T) {
	goBody := `{"data":[{"id":"deepseek-v4-pro","object":"model","created":1,"owned_by":"test"}]}`
	zenBody := `{"data":[{"id":"deepseek-v4-pro","object":"model","created":1,"owned_by":"test"}]}`
	orBody := `{"data":[{"id":"deepseek/deepseek-v4-pro","name":"DeepSeek V4 Pro","pricing":{"prompt":"0.000000435","completion":"0.000001"},"context_length":1000000,"benchmarks":{"artificial_analysis":{"intelligence_index":44.3}}}]}`

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

	// Zero hours should default to 8.
	result, err := EstimateCost(context.Background(), client, 0, []string{"orchestrator"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.HoursPerDay != 8 {
		t.Errorf("HoursPerDay = %v, want 8 (default)", result.HoursPerDay)
	}
}

func TestEstimateCost_MissingOCKey(t *testing.T) {
	client := &Client{OCAPIKey: "", ORAPIKey: ""}
	_, err := EstimateCost(context.Background(), client, 8, nil)
	if err == nil {
		t.Fatal("expected error for missing OC key")
	}
}

func TestFormatCostResult(t *testing.T) {
	result := &CostResult{
		Estimates: []CostEstimate{
			{
				Agent:            "orchestrator",
				Model:            "DeepSeek V4 Pro",
				Provider:         "DeepSeek",
				TokensPerSession: TokenUsage{Input: 50_000, Output: 10_000},
				CostPerSession:   0.018,
				MonthlyCost:      3.20,
				SessionsPerMonth: 176,
			},
		},
		TotalMonthly: 3.20,
		ByProvider:   map[string]float64{"DeepSeek": 3.20},
		HoursPerDay:  8,
		DaysPerMonth: 22,
	}

	text := FormatCostResult(result)

	if !contains(text, "8 horas/día") {
		t.Errorf("missing hours per day in output")
	}
	if !contains(text, "orchestrator (DeepSeek V4 Pro)") {
		t.Errorf("missing agent and model in output")
	}
	if !contains(text, "$3.20/mes") {
		t.Errorf("missing monthly cost in output")
	}
	if !contains(text, "50K input + 10K output") {
		t.Errorf("missing token breakdown in output")
	}
	if !contains(text, "Total estimado") {
		t.Errorf("missing total in output")
	}
	if !contains(text, "Desglose por provider") {
		t.Errorf("missing provider breakdown in output")
	}
}
