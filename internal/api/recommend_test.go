package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/R0LM0/somm/v2/internal/profile"
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

	best := findBestModel([]EnrichedModel{a, b}, role, map[string]int{}, map[string]string{})
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

	best := findBestModel([]EnrichedModel{a, b}, role, map[string]int{}, map[string]string{})
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

	best := findBestModel([]EnrichedModel{a, b}, role, map[string]int{}, map[string]string{})
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

	best := findBestModel([]EnrichedModel{pricey, cheap}, role, map[string]int{}, map[string]string{})
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

	best := findBestModel([]EnrichedModel{zModel, aModel}, role, map[string]int{}, map[string]string{})
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

	best := findBestModel([]EnrichedModel{aboveCeiling, withinCeiling}, role, map[string]int{}, map[string]string{})
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

	got := buildReason(role, best)
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

	got := buildReason(role, best)
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

	got := buildReason(role, best)
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

	candidates := collectCandidates(models, role, map[string]int{}, 2, map[string]string{})
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

	candidates := collectCandidates(models, roleB, map[string]int{}, 2, assignedFamily)
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

	best := findBestModel(models, role, used, map[string]string{})
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

	best := findBestModel(models, role, map[string]int{}, map[string]string{})
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

	best := findBestModel(models, role, map[string]int{}, map[string]string{})
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

	best := findBestModel(models, role, map[string]int{}, map[string]string{})
	if best == nil || best.model.OCID != "null-intel" {
		t.Fatalf("expected null-intelligence (50.0 fallback) to win the tiebreak, got %+v", best)
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

	candidates := collectCandidates(models, role, map[string]int{}, 2, map[string]string{})
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
