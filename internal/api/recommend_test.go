package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/R0LM0/somm/v2/internal/profile"
	"github.com/R0LM0/somm/v2/internal/profile/plans"
)

// mkModel builds an EnrichedModel for scoring-level unit tests. pricePerM is
// the input price per 1M tokens (matching how prices are quoted elsewhere in
// this package); it is stored internally as a per-token price, mirroring
// EnrichedModel.Pricing.Prompt semantics.
func mkModel(ocID string, intelligence, coding, agentic *float64, pricePerM float64, contextLen int64) EnrichedModel {
	return EnrichedModel{
		OCID:         ocID,
		OCName:       ocID,
		Subscription: "both",
		Pricing: &Money{
			Prompt:     pricePerM / 1_000_000,
			Completion: pricePerM * 2 / 1_000_000,
		},
		ContextLength: int64Ptr(contextLen),
		Benchmarks:    ModelBenchmarks{Intelligence: intelligence, Coding: coding, Agentic: agentic},
	}
}

func mustPreset(t *testing.T) *profile.Profile {
	t.Helper()
	prof, err := profile.Preset()
	if err != nil {
		t.Fatalf("loading embedded gentle-ai preset: %v", err)
	}
	return prof
}

func TestRecommendConfig_MissingOCKey(t *testing.T) {
	client := &Client{OCAPIKey: "", ORAPIKey: ""}
	_, _, err := RecommendConfig(context.Background(), client, mustPreset(t), nil)
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

	providers, recs, err := RecommendConfig(context.Background(), client, mustPreset(t), nil)
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

	_, recs, err := RecommendConfig(context.Background(), client, mustPreset(t), []string{"sdd-apply", "sdd-verify"})
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

	_, _, err := RecommendConfig(context.Background(), client, mustPreset(t), []string{"nonexistent-role"})
	if err == nil {
		t.Fatal("expected error for invalid filter, got nil")
	}
}

func TestFilterRoles(t *testing.T) {
	all := mustPreset(t).Roles

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
			Agent:        "orchestrator",
			Criticidad:   "CRÍTICO",
			Model:        "DeepSeek V4 Pro",
			Provider:     "DeepSeek",
			PriceInput:   0.435,
			PriceOutput:  1.0,
			Intelligence: &intelligence,
			Reason:       "Mejor relación calidad/precio",
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

// --- Phase 6: weighted-scoring unit tests ---

// TestFindBestModel_RawRatioOrdering locks in the design's worked example
// (Decision 1): the raw ratio must pick candidate B, not the
// normalize-then-ratio winner A.
func TestFindBestModel_RawRatioOrdering(t *testing.T) {
	role := profile.Role{ID: "r", Weights: map[string]float64{profile.MetricIntelligence: 1.0}}
	a := mkModel("model-a", float64Ptr(72.5), nil, nil, 30, 200000)
	b := mkModel("model-b", float64Ptr(52.1), nil, nil, 0.95, 200000)

	best := findBestModel([]EnrichedModel{a, b}, role, map[string]int{}, map[string]string{}, nil)
	if best == nil {
		t.Fatal("expected a candidate")
	}
	if best.model.OCID != "model-b" {
		t.Errorf("winner = %s, want model-b (raw ratio: A=2.417, B=54.8)", best.model.OCID)
	}
}

// TestFindBestModel_DefaultObjectiveMatchesValueRanking is an approval test
// (strict-tdd refactoring pattern) locking in current findBestModel
// behavior before the objective switch is introduced (design Decision 1 &
// 4, task 1.5): a role with no Selection resolved (nil, as when a Role is
// hand-built without going through profile.Load) MUST rank exactly like
// the pre-refactor "value" objective — same worked example as
// TestFindBestModel_RawRatioOrdering.
func TestFindBestModel_DefaultObjectiveMatchesValueRanking(t *testing.T) {
	role := profile.Role{ID: "r", Weights: map[string]float64{profile.MetricIntelligence: 1.0}}
	if role.Selection != nil {
		t.Fatal("test setup invariant broken: role.Selection must be nil here")
	}
	a := mkModel("model-a", float64Ptr(72.5), nil, nil, 30, 200000)
	b := mkModel("model-b", float64Ptr(52.1), nil, nil, 0.95, 200000)

	best := findBestModel([]EnrichedModel{a, b}, role, map[string]int{}, map[string]string{}, nil)
	if best == nil {
		t.Fatal("expected a candidate")
	}
	if best.model.OCID != "model-b" {
		t.Errorf("winner = %s, want model-b (raw ratio: A=2.417, B=54.8), unchanged from pre-refactor behavior", best.model.OCID)
	}
}

// TestFindBestModel_QualityObjectivePicksTopQraw locks in the
// weighted-scoring spec scenario "quality objective picks top-Qraw
// candidate over better ratio" (design Decision 4's worked example).
func TestFindBestModel_QualityObjectivePicksTopQraw(t *testing.T) {
	role := profile.Role{
		ID:        "r",
		Weights:   map[string]float64{profile.MetricIntelligence: 1.0},
		Selection: &profile.Selection{Objective: profile.ObjectiveQuality, Currency: profile.CurrencyUSD},
	}
	a := mkModel("model-a", float64Ptr(90), nil, nil, 50, 200000) // Qraw=90, price=50
	b := mkModel("model-b", float64Ptr(80), nil, nil, 5, 200000)  // Qraw=80, price=5 (better ratio)

	best := findBestModel([]EnrichedModel{a, b}, role, map[string]int{}, map[string]string{}, nil)
	if best == nil {
		t.Fatal("expected a candidate")
	}
	if best.model.OCID != "model-a" {
		t.Errorf("winner = %s, want model-a (top Qraw=90 despite worse ratio)", best.model.OCID)
	}
}

// TestFindBestModel_QualityObjectiveTiesBrokenByLowerPrice locks in the
// weighted-scoring spec scenario "quality objective ties broken by lower
// denominator".
func TestFindBestModel_QualityObjectiveTiesBrokenByLowerPrice(t *testing.T) {
	role := profile.Role{
		ID:        "r",
		Weights:   map[string]float64{profile.MetricIntelligence: 1.0},
		Selection: &profile.Selection{Objective: profile.ObjectiveQuality, Currency: profile.CurrencyUSD},
	}
	cheap := mkModel("cheap", float64Ptr(70), nil, nil, 2, 200000)
	pricey := mkModel("pricey", float64Ptr(70), nil, nil, 20, 200000)

	best := findBestModel([]EnrichedModel{pricey, cheap}, role, map[string]int{}, map[string]string{}, nil)
	if best == nil {
		t.Fatal("expected a candidate")
	}
	if best.model.OCID != "cheap" {
		t.Errorf("winner = %s, want cheap (equal Qraw=70, tie broken by lower price)", best.model.OCID)
	}
}

// TestFindBestModel_QualityObjectiveOCIDTiebreakForFullTie proves the total
// order design Decision 4 requires: when Qraw AND price both tie,
// sort.Slice (unstable) would otherwise make the winner nondeterministic —
// the OCID ascending tiebreak fixes it.
func TestFindBestModel_QualityObjectiveOCIDTiebreakForFullTie(t *testing.T) {
	role := profile.Role{
		ID:        "r",
		Weights:   map[string]float64{profile.MetricIntelligence: 1.0},
		Selection: &profile.Selection{Objective: profile.ObjectiveQuality, Currency: profile.CurrencyUSD},
	}
	zModel := mkModel("z-model", float64Ptr(70), nil, nil, 5, 200000)
	aModel := mkModel("a-model", float64Ptr(70), nil, nil, 5, 200000)

	best := findBestModel([]EnrichedModel{zModel, aModel}, role, map[string]int{}, map[string]string{}, nil)
	if best == nil {
		t.Fatal("expected a candidate")
	}
	if best.model.OCID != "a-model" {
		t.Errorf("winner = %s, want a-model (fully tied on Qraw and price, lower OCID wins deterministically)", best.model.OCID)
	}
}

// TestFindBestModel_BudgetObjectiveNeverExceedsCeiling locks in the
// weighted-scoring spec scenario "budget objective never exceeds
// max_input_price": the best-ratio candidate above the ceiling MUST be
// excluded, and the winner MUST be the best value-ranked candidate at or
// below it.
func TestFindBestModel_BudgetObjectiveNeverExceedsCeiling(t *testing.T) {
	ceiling := 5.0
	role := profile.Role{
		ID:            "r",
		Weights:       map[string]float64{profile.MetricIntelligence: 1.0},
		MaxInputPrice: &ceiling,
		Selection:     &profile.Selection{Objective: profile.ObjectiveBudget, Currency: profile.CurrencyUSD},
	}
	// Best Qraw/price ratio (80/1=80) but above the 5.0 ceiling.
	aboveCeiling := mkModel("above-ceiling", float64Ptr(80), nil, nil, 8, 200000)
	// Worse ratio (60/4=15) but at/below the ceiling — must win under budget.
	withinCeiling := mkModel("within-ceiling", float64Ptr(60), nil, nil, 4, 200000)

	best := findBestModel([]EnrichedModel{aboveCeiling, withinCeiling}, role, map[string]int{}, map[string]string{}, nil)
	if best == nil {
		t.Fatal("expected a candidate")
	}
	if best.model.OCID != "within-ceiling" {
		t.Errorf("winner = %s, want within-ceiling (above-ceiling candidate must be excluded despite the better ratio)", best.model.OCID)
	}
}

// TestBuildReason_ValueObjectiveUnchanged is an approval test locking in
// the exact pre-refactor Reason format for the "value" objective (design
// Decision 6: this arm must stay byte-identical).
func TestBuildReason_ValueObjectiveUnchanged(t *testing.T) {
	role := profile.Role{ID: "r", Description: "role desc", Weights: map[string]float64{profile.MetricIntelligence: 1.0}}
	m := mkModel("model-a", float64Ptr(72.5), nil, nil, 30, 200000)
	best := &scoredModel{model: m, qraw: 72.5, price: 30}

	got := buildReason(role, best, nil)
	want := "Mejor relación calidad/precio: 72.5 intelligence, $30.000/M input, 200K ctx (Go + Zen) — role desc"
	if got != want {
		t.Errorf("buildReason() = %q, want %q", got, want)
	}
}

// TestBuildReason_QualityObjective locks in design Decision 6's quality+usd
// Reason arm.
func TestBuildReason_QualityObjective(t *testing.T) {
	role := profile.Role{
		ID:          "r",
		Description: "role desc",
		Weights:     map[string]float64{profile.MetricIntelligence: 1.0},
		Selection:   &profile.Selection{Objective: profile.ObjectiveQuality, Currency: profile.CurrencyUSD},
	}
	m := mkModel("model-a", float64Ptr(90), nil, nil, 50, 200000)
	best := &scoredModel{model: m, qraw: 90, price: 50}

	got := buildReason(role, best, nil)
	if !strings.Contains(got, "Máxima calidad disponible") {
		t.Errorf("buildReason() = %q, want it to start with the quality-objective prefix", got)
	}
	if !strings.Contains(got, "90.0 intelligence") || !strings.Contains(got, "$50.000/M input") {
		t.Errorf("buildReason() = %q, want it to interpolate metric and price", got)
	}
}

// TestBuildReason_BudgetObjective locks in design Decision 6's budget+usd
// Reason arm, including the effective ceiling in the text.
func TestBuildReason_BudgetObjective(t *testing.T) {
	ceiling := 5.0
	role := profile.Role{
		ID:            "r",
		Description:   "role desc",
		Weights:       map[string]float64{profile.MetricIntelligence: 1.0},
		MaxInputPrice: &ceiling,
		Selection:     &profile.Selection{Objective: profile.ObjectiveBudget, Currency: profile.CurrencyUSD},
	}
	m := mkModel("model-a", float64Ptr(60), nil, nil, 4, 200000)
	best := &scoredModel{model: m, qraw: 60, price: 4}

	got := buildReason(role, best, nil)
	if !strings.Contains(got, "$5.00/M") {
		t.Errorf("buildReason() = %q, want it to contain the effective ceiling $5.00/M", got)
	}
	if !strings.Contains(got, "60.0 intelligence") || !strings.Contains(got, "$4.000/M input") {
		t.Errorf("buildReason() = %q, want it to interpolate metric and price", got)
	}
}

func TestCollectCandidates_HardConstraints(t *testing.T) {
	minCtx := int64(100000)
	maxPrice := 5.0
	role := profile.Role{
		ID:            "r",
		Weights:       map[string]float64{profile.MetricIntelligence: 1.0},
		MinContext:    &minCtx,
		MaxInputPrice: &maxPrice,
	}
	models := []EnrichedModel{
		mkModel("both-ok", float64Ptr(50), nil, nil, 2.0, 200000),     // passes both bounds
		mkModel("low-ctx", float64Ptr(50), nil, nil, 2.0, 50000),      // fails min_context
		mkModel("high-price", float64Ptr(50), nil, nil, 10.0, 200000), // fails max_input_price
		mkModel("both-fail", float64Ptr(50), nil, nil, 10.0, 50000),   // fails both
	}

	candidates := collectCandidates(models, role, map[string]int{}, 2, map[string]string{}, nil)
	if len(candidates) != 1 || candidates[0].model.OCID != "both-ok" {
		t.Fatalf("expected only both-ok to remain, got %+v", candidates)
	}
}

func TestCollectCandidates_ExcludeFamilyOf(t *testing.T) {
	roleB := profile.Role{ID: "B", Weights: map[string]float64{profile.MetricIntelligence: 1.0}, ExcludeFamilyOf: "A"}
	models := []EnrichedModel{
		mkModel("gpt-5.5-pro", float64Ptr(80), nil, nil, 2.0, 200000),
		mkModel("deepseek-v4-pro", float64Ptr(50), nil, nil, 2.0, 200000),
	}
	assignedFamily := map[string]string{"A": "OpenAI"}

	candidates := collectCandidates(models, roleB, map[string]int{}, 2, assignedFamily, nil)
	for _, c := range candidates {
		if deriveProvider(c.model.OCID) == "OpenAI" {
			t.Errorf("expected OpenAI family excluded, got %s", c.model.OCID)
		}
	}
	if len(candidates) != 1 || candidates[0].model.OCID != "deepseek-v4-pro" {
		t.Fatalf("unexpected candidates: %+v", candidates)
	}
}

func TestComputeNormalized_SingleCandidateIsNeutral(t *testing.T) {
	role := profile.Role{ID: "r", Weights: map[string]float64{profile.MetricIntelligence: 0.6, profile.MetricCoding: 0.4}}
	candidates := []scoredModel{
		{model: mkModel("only", float64Ptr(70), float64Ptr(60), nil, 1, 100000)},
	}

	computeNormalized(role, candidates)

	want := 0.6*1.0 + 0.4*1.0
	if diff := candidates[0].qnorm - want; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("single-candidate qnorm = %v, want %v (neutral 1.0 on every metric)", candidates[0].qnorm, want)
	}
}

func TestComputeNormalized_AllEqualIsNeutral(t *testing.T) {
	role := profile.Role{ID: "r", Weights: map[string]float64{profile.MetricIntelligence: 0.6, profile.MetricCoding: 0.4}}
	candidates := []scoredModel{
		{model: mkModel("m1", float64Ptr(70), float64Ptr(50), nil, 1, 100000)},
		{model: mkModel("m2", float64Ptr(70), float64Ptr(80), nil, 1, 100000)},
		{model: mkModel("m3", float64Ptr(70), float64Ptr(30), nil, 1, 100000)},
	}

	computeNormalized(role, candidates)

	// intelligence is identical (70) across all three candidates, so it must
	// normalize to 1.0 (neutral, no division by zero) and contribute exactly
	// 0.6 to every candidate's qnorm regardless of the varying coding metric.
	for _, c := range candidates {
		if c.qnorm < 0.6-1e-9 {
			t.Errorf("%s: qnorm = %v, expected >= 0.6 (equal-metric contributes neutral 0.6)", c.model.OCID, c.qnorm)
		}
	}
}

func TestFindBestModel_CapRelaxationRecoversCandidate(t *testing.T) {
	role := profile.Role{ID: "r", Weights: map[string]float64{profile.MetricIntelligence: 1.0}}
	models := []EnrichedModel{mkModel("m1", float64Ptr(50), nil, nil, 1, 100000)}
	used := map[string]int{"m1": 2} // already at cap-2

	best := findBestModel(models, role, used, map[string]string{}, nil)
	if best == nil {
		t.Fatal("expected relaxation to cap-3 to recover the candidate")
	}
	if best.model.OCID != "m1" {
		t.Errorf("got %s, want m1", best.model.OCID)
	}
}

func TestFindBestModel_ConstraintsAloneNeverRelax(t *testing.T) {
	minCtx := int64(1_000_000)
	role := profile.Role{ID: "r", Weights: map[string]float64{profile.MetricIntelligence: 1.0}, MinContext: &minCtx}
	models := []EnrichedModel{mkModel("m1", float64Ptr(50), nil, nil, 1, 100000)}

	best := findBestModel(models, role, map[string]int{}, map[string]string{}, nil)
	if best != nil {
		t.Errorf("expected no candidate (hard constraint unmet even after relaxation), got %+v", best)
	}
}

func TestRecommendConfig_AssignmentCapAtThree(t *testing.T) {
	goBody := `{"data":[{"id":"deepseek-v4-pro","object":"model","created":1,"owned_by":"test"}]}`
	orBody := `{"data":[{"id":"deepseek/deepseek-v4-pro","name":"DeepSeek V4 Pro","pricing":{"prompt":"0.000000435","completion":"0.000001"},"context_length":1000000,"benchmarks":{"artificial_analysis":{"intelligence_index":44.3,"coding_index":38.2,"agentic_index":41.5}}}]}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/zen/go/v1/models", "/zen/v1/models":
			w.Write([]byte(goBody))
		case "/api/v1/models":
			w.Write([]byte(orBody))
		}
	}))
	defer srv.Close()

	client := &Client{HTTPClient: rewriteTransport(srv), OCAPIKey: "test-key", ORAPIKey: "test-or-key"}

	_, recs, err := RecommendConfig(context.Background(), client, mustPreset(t), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	counts := map[string]int{}
	for _, r := range recs {
		if r.OCID != "" {
			counts[r.OCID]++
		}
	}
	for id, c := range counts {
		if c > 3 {
			t.Errorf("model %s assigned %d times, want <=3", id, c)
		}
	}
	if counts["deepseek-v4-pro"] != 3 {
		t.Errorf("expected exactly 3 assignments (cap after relaxation), got %d", counts["deepseek-v4-pro"])
	}
}

func TestFindBestModel_EmptyWeightsSortsByPriceAscending(t *testing.T) {
	role := profile.Role{ID: "cheap", Weights: map[string]float64{}}
	models := []EnrichedModel{
		mkModel("expensive", float64Ptr(90), nil, nil, 10, 100000),
		mkModel("cheap-model", float64Ptr(30), nil, nil, 1, 100000),
	}

	best := findBestModel(models, role, map[string]int{}, map[string]string{}, nil)
	if best == nil || best.model.OCID != "cheap-model" {
		t.Fatalf("expected cheapest model to win, got %+v", best)
	}
}

func TestFindBestModel_EmptyWeightsNullIntelligenceFallsBackTo50(t *testing.T) {
	role := profile.Role{ID: "cheap", Weights: map[string]float64{}}
	models := []EnrichedModel{
		// Tied on price; null intelligence resolves to 50.0 and must beat 40.0.
		mkModel("null-intel", nil, nil, nil, 1, 100000),
		mkModel("forty-intel", float64Ptr(40), nil, nil, 1, 100000),
	}

	best := findBestModel(models, role, map[string]int{}, map[string]string{}, nil)
	if best == nil || best.model.OCID != "null-intel" {
		t.Fatalf("expected null-intelligence (50.0 fallback) to win the tiebreak, got %+v", best)
	}
}

// --- Phase 4 (PR2a): plans, quota, frequency unit tests ---

// TestFindBestModel_QuotaCurrencyRanksHighQuotaAboveLowQuotaWinner locks in
// design Decision 2/3's exact P/C worked example: at frequency: high, C
// (cheap-abundant) wins; at frequency: low, the SAME two candidates invert
// so P (premium) wins. This inversion is the load-bearing proof that the
// saturation cap (H_sat=200) makes frequency actually reorder candidates
// (weighted-scoring spec, "quota currency ranks high-quota cheap model
// above low-quota premium model"; plan-quota-currency spec, "Higher
// frequency weight favors quota-abundant models").
func TestFindBestModel_QuotaCurrencyRanksHighQuotaAboveLowQuotaWinner(t *testing.T) {
	// ModelPlan.Shape{Input: 1} makes cost_per_request == price/1e6, so
	// TierUSD = wantRp5h * price / 200000 reproduces the original
	// map[string]int{"cand-p": 110, "cand-c": 45300} derived requests/5h
	// under each candidate's mkModel price (design: price-aware Requests
	// replaces the old static-int schema; task 3.15).
	plansTable := &plans.Table{
		Plan:      "test",
		CuratedAt: "2026-01-01",
		Models: map[string]plans.ModelPlan{
			"cand-p": {Shape: plans.Shape{Input: 1}, TierUSD: 110 * 3.00 / 200000},
			"cand-c": {Shape: plans.Shape{Input: 1}, TierUSD: 45300 * 0.15 / 200000},
		},
	}
	p := mkModel("cand-p", float64Ptr(80), nil, nil, 3.00, 200000)
	c := mkModel("cand-c", float64Ptr(50), nil, nil, 0.15, 200000)

	t.Run("frequency: high -> C wins (cheap-abundant beats premium low-quota)", func(t *testing.T) {
		role := profile.Role{
			ID:        "r",
			Weights:   map[string]float64{profile.MetricIntelligence: 1.0},
			Selection: &profile.Selection{Objective: profile.ObjectiveValue, Currency: profile.CurrencyQuota},
			Frequency: profile.FrequencyHigh,
		}
		best := findBestModel([]EnrichedModel{p, c}, role, map[string]int{}, map[string]string{}, plansTable)
		if best == nil {
			t.Fatal("expected a candidate")
		}
		if best.model.OCID != "cand-c" {
			t.Errorf("winner = %s, want cand-c (score ~50.00 vs cand-p ~11.00 at frequency: high)", best.model.OCID)
		}
	})

	t.Run("frequency: low -> P wins, inverting the high-frequency winner", func(t *testing.T) {
		role := profile.Role{
			ID:        "r",
			Weights:   map[string]float64{profile.MetricIntelligence: 1.0},
			Selection: &profile.Selection{Objective: profile.ObjectiveValue, Currency: profile.CurrencyQuota},
			Frequency: profile.FrequencyLow,
		}
		best := findBestModel([]EnrichedModel{p, c}, role, map[string]int{}, map[string]string{}, plansTable)
		if best == nil {
			t.Fatal("expected a candidate")
		}
		if best.model.OCID != "cand-p" {
			t.Errorf("winner = %s, want cand-p (score 80.00 vs cand-c 50.00 at frequency: low)", best.model.OCID)
		}
	})
}

// TestCollectCandidates_QuotaUntabulatedFallbackUsesPriceOverPMinBridge
// locks in plan-quota-currency spec's "Untabulated model still receives a
// comparable rank" scenario: P_min=2.0 over the constraint-filtered set,
// untabulated candidate priced 4.0 -> denominator = 4.0/2.0 = 2.0, not the
// raw usd price.
func TestCollectCandidates_QuotaUntabulatedFallbackUsesPriceOverPMinBridge(t *testing.T) {
	role := profile.Role{
		ID:        "r",
		Weights:   map[string]float64{profile.MetricIntelligence: 1.0},
		Selection: &profile.Selection{Objective: profile.ObjectiveValue, Currency: profile.CurrencyQuota},
	}
	plansTable := &plans.Table{Plan: "test", CuratedAt: "2026-01-01", Models: map[string]plans.ModelPlan{}}
	cheapest := mkModel("cheapest", float64Ptr(40), nil, nil, 2.0, 200000) // P_min = 2.0
	untabulated := mkModel("untabulated", float64Ptr(65), nil, nil, 4.0, 200000)

	candidates := collectCandidates([]EnrichedModel{cheapest, untabulated}, role, map[string]int{}, 2, map[string]string{}, plansTable)

	byID := make(map[string]scoredModel, len(candidates))
	for _, c := range candidates {
		byID[c.model.OCID] = c
	}

	u, ok := byID["untabulated"]
	if !ok {
		t.Fatal("expected untabulated candidate to be present, not excluded")
	}
	if u.hasQuota {
		t.Error("expected hasQuota=false for a candidate absent from the quota table")
	}
	if u.quota != 2.0 {
		t.Errorf("untabulated denominator = %v, want 2.0 (price 4.0 / P_min 2.0)", u.quota)
	}
}

// TestPriceOf_MapsEnrichedModelPricing locks in priceOf's projection from
// EnrichedModel.Pricing into plans.Price (design: plans never imports api;
// Price is the seam), including the D5 nil-vs-present distinction for
// InputCacheRead.
func TestPriceOf_MapsEnrichedModelPricing(t *testing.T) {
	t.Run("cache-read price present -> copied, scaled to per-1M", func(t *testing.T) {
		m := EnrichedModel{
			Pricing: &Money{
				Prompt:         0.000002,
				Completion:     0.000006,
				InputCacheRead: float64Ptr(0.0000003),
			},
		}
		got := priceOf(m)
		if got.InputPerM != 2.0 {
			t.Errorf("InputPerM = %v, want 2.0", got.InputPerM)
		}
		if got.OutputPerM != 6.0 {
			t.Errorf("OutputPerM = %v, want 6.0", got.OutputPerM)
		}
		if got.CacheReadPerM == nil || *got.CacheReadPerM != 0.3 {
			t.Errorf("CacheReadPerM = %v, want 0.3", got.CacheReadPerM)
		}
	})

	t.Run("cache-read price absent -> nil, never a zero-as-default", func(t *testing.T) {
		m := EnrichedModel{
			Pricing: &Money{Prompt: 0.000002, Completion: 0.000006},
		}
		got := priceOf(m)
		if got.CacheReadPerM != nil {
			t.Errorf("CacheReadPerM = %v, want nil", got.CacheReadPerM)
		}
	})

	t.Run("nil Pricing -> zero Price, no panic", func(t *testing.T) {
		got := priceOf(EnrichedModel{})
		if got.InputPerM != 0 || got.OutputPerM != 0 || got.CacheReadPerM != nil {
			t.Errorf("priceOf(nil Pricing) = %+v, want zero Price", got)
		}
	})
}

// TestCollectCandidates_CuratedNilPriceExcludedNoPanic locks in design D3:
// a candidate curated in the plans table but with a nil Pricing never
// reaches resolveQuotaDenominators or the price/P_min fallback bridge — the
// existing nil-price hard-constraint gate in collectCandidates excludes it
// before any quota-currency logic runs, so it never panics and never
// silently appears in the candidate set.
func TestCollectCandidates_CuratedNilPriceExcludedNoPanic(t *testing.T) {
	role := profile.Role{
		ID:        "r",
		Weights:   map[string]float64{profile.MetricIntelligence: 1.0},
		Selection: &profile.Selection{Objective: profile.ObjectiveValue, Currency: profile.CurrencyQuota},
	}
	plansTable := &plans.Table{
		Plan:      "test",
		CuratedAt: "2026-01-01",
		Models: map[string]plans.ModelPlan{
			"muse-spark-1.2-contributor": {Shape: plans.Shape{Input: 620, Cached: 71400, Output: 300}, CacheReadRatio: 0.02, TierUSD: 60},
		},
	}
	noPriceModel := EnrichedModel{
		OCID:       "muse-spark-1.2-contributor",
		OCName:     "muse-spark-1.2-contributor",
		Pricing:    nil, // curated in the table, but no live price source match (design D3)
		Benchmarks: ModelBenchmarks{Intelligence: float64Ptr(50)},
	}
	priced := mkModel("priced-model", float64Ptr(60), nil, nil, 3.0, 200000)

	var candidates []scoredModel
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("collectCandidates panicked on a curated-but-nil-price model: %v", r)
			}
		}()
		candidates = collectCandidates([]EnrichedModel{noPriceModel, priced}, role, map[string]int{}, 2, map[string]string{}, plansTable)
	}()

	for _, c := range candidates {
		if c.model.OCID == "muse-spark-1.2-contributor" {
			t.Fatal("expected curated-but-nil-price model to be absent from candidates, found it")
		}
	}
	if len(candidates) != 1 || candidates[0].model.OCID != "priced-model" {
		t.Fatalf("expected exactly the priced model to remain a candidate, got %+v", candidates)
	}
}

// TestFindBestModel_FrequencyHasNoEffectUnderUSDCurrency locks in
// role-profiles spec's "Frequency ignored under usd currency" scenario:
// changing frequency must not change the winner (or anything) when the
// effective currency is usd.
func TestFindBestModel_FrequencyHasNoEffectUnderUSDCurrency(t *testing.T) {
	a := mkModel("model-a", float64Ptr(72.5), nil, nil, 30, 200000)
	b := mkModel("model-b", float64Ptr(52.1), nil, nil, 0.95, 200000)

	roleHigh := profile.Role{ID: "r", Weights: map[string]float64{profile.MetricIntelligence: 1.0}, Frequency: profile.FrequencyHigh}
	roleLow := profile.Role{ID: "r", Weights: map[string]float64{profile.MetricIntelligence: 1.0}, Frequency: profile.FrequencyLow}

	bestHigh := findBestModel([]EnrichedModel{a, b}, roleHigh, map[string]int{}, map[string]string{}, nil)
	bestLow := findBestModel([]EnrichedModel{a, b}, roleLow, map[string]int{}, map[string]string{}, nil)

	if bestHigh == nil || bestLow == nil {
		t.Fatal("expected candidates in both cases")
	}
	if bestHigh.model.OCID != bestLow.model.OCID {
		t.Errorf("frequency changed the winner under usd currency: high=%s low=%s", bestHigh.model.OCID, bestLow.model.OCID)
	}
	if bestHigh.model.OCID != "model-b" {
		t.Errorf("winner = %s, want model-b (raw ratio unaffected by frequency under usd)", bestHigh.model.OCID)
	}
}

// TestBuildReason_QuotaTabulatedContainsCuratedDate locks in
// plan-quota-derivation's Staleness Surfacing requirement: the reason
// carries the table's curation date, phrased as when the shape/tier inputs
// were curated (never as a measurement of the shown request count, since
// that count is now derived at request time from live pricing).
func TestBuildReason_QuotaTabulatedContainsCuratedDate(t *testing.T) {
	role := profile.Role{
		ID:          "r",
		Description: "role desc",
		Weights:     map[string]float64{profile.MetricIntelligence: 1.0},
		Selection:   &profile.Selection{Objective: profile.ObjectiveValue, Currency: profile.CurrencyQuota},
	}
	// buildReason now reads best.reqPer5H directly (cached by
	// resolveQuotaDenominators) instead of re-deriving via
	// plansTable.Requests, so the table itself no longer needs a "model-a"
	// entry — only CuratedAt and a non-nil table matter here (task 3.17).
	plansTable := &plans.Table{Plan: "test", CuratedAt: "2026-08-01", Models: map[string]plans.ModelPlan{}}
	m := mkModel("model-a", float64Ptr(80), nil, nil, 3.0, 200000)
	best := &scoredModel{model: m, qraw: 80, price: 3.0, quota: 7.27, hasQuota: true, reqPer5H: 110}

	got := buildReason(role, best, plansTable)
	if !strings.Contains(got, "2026-08-01") {
		t.Errorf("buildReason() = %q, want it to contain the curation date 2026-08-01", got)
	}
	if !strings.Contains(got, "110 req/5h") {
		t.Errorf("buildReason() = %q, want it to contain the requests_per_5h quota", got)
	}
	if !strings.Contains(got, "derivado del precio actual") {
		t.Errorf("buildReason() = %q, want it to state the quota was derived from the current price", got)
	}
	if strings.Contains(got, "medido") {
		t.Errorf("buildReason() = %q, must NOT claim the request count was measured", got)
	}
}

// TestBuildReason_QuotaFallbackOmitsCuratedDate locks in
// plan-quota-derivation's Staleness Surfacing requirement: a price-fallback
// winner's reason omits the curation date and any curated/measured quota
// claim entirely.
func TestBuildReason_QuotaFallbackOmitsCuratedDate(t *testing.T) {
	role := profile.Role{
		ID:          "r",
		Description: "role desc",
		Weights:     map[string]float64{profile.MetricIntelligence: 1.0},
		Selection:   &profile.Selection{Objective: profile.ObjectiveValue, Currency: profile.CurrencyQuota},
	}
	plansTable := &plans.Table{Plan: "test", CuratedAt: "2026-08-01", Models: map[string]plans.ModelPlan{}}
	m := mkModel("model-untabulated", float64Ptr(65), nil, nil, 4.0, 200000)
	best := &scoredModel{model: m, qraw: 65, price: 4.0, quota: 2.0, hasQuota: false}

	got := buildReason(role, best, plansTable)
	if strings.Contains(got, "2026-08-01") {
		t.Errorf("buildReason() = %q, must NOT contain a curation date for a fallback winner", got)
	}
	if !strings.Contains(got, "sin datos de cuota") {
		t.Errorf("buildReason() = %q, want it to explicitly mark the fallback (no quota data)", got)
	}
	if strings.Contains(got, "medido") {
		t.Errorf("buildReason() = %q, must NOT use \"medido\" wording", got)
	}
}

// --- Phase 2 (live-quota-derivation): characterization tests pinning
// resolveQuotaDenominators' current behavior against the still-static
// map[string]int table, as a RED safety net ahead of Phase 3's price-aware
// rewrite. These are approval tests: they describe today's reality, not a
// new requirement, so they must PASS immediately (strict-tdd.md, "Approval
// Testing").

// TestResolveQuotaDenominators_SaturationCapAndHeadroom locks in the
// saturating affordability formula
// H_sat / min(requests_per_5h / frequency_weight, H_sat) directly, isolating
// the saturation-cap boundary and the frequency-weighted threshold crossing
// that the 5 existing indirect quota tests (via findBestModel/buildReason)
// don't exercise in isolation.
func TestResolveQuotaDenominators_SaturationCapAndHeadroom(t *testing.T) {
	tests := []struct {
		name          string
		frequency     string
		requestsPer5h int
		wantQuota     float64
	}{
		{"saturating: requests well above H_sat cap yields quota 1", "", 300, 1.0},
		{"exactly at H_sat boundary: cap does not engage but quota is still 1", "", 200, 1.0},
		{"scarce: headroom below H_sat yields quota above 1", "", 50, 4.0},
		{"high frequency weight pushes rp5h above threshold: saturates", profile.FrequencyHigh, 1000, 1.0},
		{"high frequency weight keeps rp5h below threshold: scarce", profile.FrequencyHigh, 400, 2.0},
		{"low frequency weight amplifies scarce headroom", profile.FrequencyLow, 40, 1.25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ocID := "model-x"
			// Shape{Input: 200000} against the model's price 5.0 makes
			// cost_per_request == 200000*5.0/1e6 == 1.0 exactly, so
			// TierUSD == requestsPer5h*5 reproduces the exact
			// requestsPer5h value the old static-int table stored (task
			// 2.1's schema is now price-derived, same scenario intent).
			plansTable := &plans.Table{
				Plan:      "test",
				CuratedAt: "2026-01-01",
				Models: map[string]plans.ModelPlan{
					ocID: {Shape: plans.Shape{Input: 200000}, TierUSD: float64(tt.requestsPer5h) * 5},
				},
			}
			model := mkModel(ocID, float64Ptr(70), nil, nil, 5.0, 200000)
			role := profile.Role{ID: "r", Frequency: tt.frequency}
			candidates := []scoredModel{{model: model, qraw: 70, price: 5.0}}

			resolveQuotaDenominators(candidates, role, plansTable)

			if !candidates[0].hasQuota {
				t.Errorf("hasQuota = false, want true for a tabulated model")
			}
			if candidates[0].quota != tt.wantQuota {
				t.Errorf("quota = %v, want %v", candidates[0].quota, tt.wantQuota)
			}
		})
	}
}

// TestResolveQuotaDenominators_UntabulatedFallback locks in the price/P_min
// bridge directly, over a candidate set that mixes a tabulated model with
// two untabulated ones, proving P_min is computed once over the whole set
// (not just the untabulated candidates).
func TestResolveQuotaDenominators_UntabulatedFallback(t *testing.T) {
	role := profile.Role{ID: "r"}
	// Shape{Input: 1000000} against the model's price 1.0 makes
	// cost_per_request == 1000000*1.0/1e6 == 1.0 exactly, so TierUSD == 500
	// reproduces the old table's exact "tabulated-model": 100 value.
	plansTable := &plans.Table{
		Plan:      "test",
		CuratedAt: "2026-01-01",
		Models: map[string]plans.ModelPlan{
			"tabulated-model": {Shape: plans.Shape{Input: 1000000}, TierUSD: 500},
		},
	}

	tabulated := mkModel("tabulated-model", float64Ptr(80), nil, nil, 1.0, 200000) // cheapest -> P_min = 1.0
	untabulatedMid := mkModel("untabulated-mid", float64Ptr(60), nil, nil, 3.0, 200000)
	untabulatedExpensive := mkModel("untabulated-expensive", float64Ptr(50), nil, nil, 5.0, 200000)

	candidates := []scoredModel{
		{model: tabulated, qraw: 80, price: 1.0},
		{model: untabulatedMid, qraw: 60, price: 3.0},
		{model: untabulatedExpensive, qraw: 50, price: 5.0},
	}

	resolveQuotaDenominators(candidates, role, plansTable)

	byID := make(map[string]scoredModel, len(candidates))
	for _, c := range candidates {
		byID[c.model.OCID] = c
	}

	if tab := byID["tabulated-model"]; !tab.hasQuota {
		t.Error("hasQuota = false for a tabulated model, want true")
	}

	mid := byID["untabulated-mid"]
	if mid.hasQuota {
		t.Error("hasQuota = true for an untabulated model, want false")
	}
	if mid.quota != 3.0 {
		t.Errorf("quota = %v, want 3.0 (price 3.0 / P_min 1.0)", mid.quota)
	}

	expensive := byID["untabulated-expensive"]
	if expensive.hasQuota {
		t.Error("hasQuota = true for an untabulated model, want false")
	}
	if expensive.quota != 5.0 {
		t.Errorf("quota = %v, want 5.0 (price 5.0 / P_min 1.0)", expensive.quota)
	}
}

// TestResolveQuotaDenominators_NilTableAllFallback locks in that a nil
// plansTable degrades every candidate to the price/P_min fallback, without
// consulting any table (the caller-facing contract collectCandidates
// depends on for a role that never resolved a table).
func TestResolveQuotaDenominators_NilTableAllFallback(t *testing.T) {
	role := profile.Role{ID: "r"}
	a := mkModel("model-a", float64Ptr(80), nil, nil, 2.0, 200000) // P_min = 2.0
	b := mkModel("model-b", float64Ptr(60), nil, nil, 6.0, 200000)

	candidates := []scoredModel{
		{model: a, qraw: 80, price: 2.0},
		{model: b, qraw: 60, price: 6.0},
	}

	resolveQuotaDenominators(candidates, role, nil)

	for _, c := range candidates {
		if c.hasQuota {
			t.Errorf("candidate %s: hasQuota = true with a nil plansTable, want false", c.model.OCID)
		}
	}
	if candidates[0].quota != 1.0 {
		t.Errorf("candidates[0].quota = %v, want 1.0 (price 2.0 / P_min 2.0)", candidates[0].quota)
	}
	if candidates[1].quota != 3.0 {
		t.Errorf("candidates[1].quota = %v, want 3.0 (price 6.0 / P_min 2.0)", candidates[1].quota)
	}
}

// TestResolveQuotaDenominators_EmptySliceNoPanic locks in that an empty
// candidate slice returns immediately without panicking (e.g. dividing by
// an infinite P_min) — collectCandidates can call this with zero
// constraint-surviving candidates.
func TestResolveQuotaDenominators_EmptySliceNoPanic(t *testing.T) {
	role := profile.Role{ID: "r"}
	plansTable := &plans.Table{Plan: "test", CuratedAt: "2026-01-01", Models: map[string]plans.ModelPlan{}}

	candidates := []scoredModel{}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("resolveQuotaDenominators panicked on an empty candidate slice: %v", r)
		}
	}()
	resolveQuotaDenominators(candidates, role, plansTable)

	if len(candidates) != 0 {
		t.Errorf("candidates length = %d, want 0 (unchanged)", len(candidates))
	}
}

// TestRecommendConfig_GoldenParity re-runs the Phase 3 fixed mock models
// through the refactored (Profile-driven) engine and asserts the output is
// byte-for-byte identical to the pre-refactor golden captured in
// testdata/gentle-ai.golden. This is the KEY parity guard for the whole
// change.
func TestRecommendConfig_GoldenParity(t *testing.T) {
	ocBody, err := os.ReadFile("testdata/gentle_ai_mock_oc.json")
	if err != nil {
		t.Fatalf("reading oc fixture: %v", err)
	}
	orBody, err := os.ReadFile("testdata/gentle_ai_mock_or.json")
	if err != nil {
		t.Fatalf("reading or fixture: %v", err)
	}
	want, err := os.ReadFile("testdata/gentle-ai.golden")
	if err != nil {
		t.Fatalf("reading golden: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/zen/go/v1/models", "/zen/v1/models":
			w.Write(ocBody)
		case "/api/v1/models":
			w.Write(orBody)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	client := &Client{HTTPClient: rewriteTransport(srv), OCAPIKey: "test-key", ORAPIKey: "test-or-key"}

	providers, recs, err := RecommendConfig(context.Background(), client, mustPreset(t), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := FormatRecommendations(providers, recs)
	if got != string(want) {
		t.Fatalf("golden parity mismatch (byte-for-byte required):\n--- got ---\n%s\n--- want ---\n%s", got, string(want))
	}
}

// TestGentleAIPreset_SingleWeightBypassesNormalization asserts every
// gentle-ai preset role has exactly one weight at 1.0 (or is a
// price-minimizing empty-weights role) and that Qraw equals the raw
// benchmark value, with normalization never engaging.
func TestGentleAIPreset_SingleWeightBypassesNormalization(t *testing.T) {
	prof := mustPreset(t)

	for _, role := range prof.Roles {
		if len(role.Weights) == 0 {
			continue // price-minimizing roles (former "cheapest") carry no weights.
		}
		if len(role.Weights) != 1 {
			t.Errorf("role %q: expected exactly 1 weight, got %d", role.ID, len(role.Weights))
			continue
		}
		for metric, w := range role.Weights {
			if w != 1.0 {
				t.Errorf("role %q: weight for %s = %v, want 1.0", role.ID, metric, w)
			}
		}
	}

	role := profile.Role{ID: "orchestrator", Weights: map[string]float64{profile.MetricIntelligence: 1.0}}
	models := []EnrichedModel{mkModel("m1", float64Ptr(72.5), nil, nil, 5, 100000)}

	candidates := collectCandidates(models, role, map[string]int{}, 2, map[string]string{}, nil)
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].qraw != 72.5 {
		t.Errorf("qraw = %v, want 72.5 (raw benchmark value)", candidates[0].qraw)
	}

	computeNormalized(role, candidates)
	if candidates[0].qnorm != 0 {
		t.Errorf("qnorm = %v, want 0 (normalization never engages for single-weight roles)", candidates[0].qnorm)
	}
}
