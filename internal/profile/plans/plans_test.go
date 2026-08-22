package plans

import (
	"math"
	"strings"
	"testing"
)

func float64Ptr(v float64) *float64 { return &v }

// --- Task 3.2: TestParseTable — valid / bad curated_at / malformed YAML ---

func TestParseTable(t *testing.T) {
	const validYAML = `
plan: test-plan
curated_at: "2026-08-01"
models:
  test-model:
    shape: {input: 100, cached: 1000, output: 50}
    cache_read_ratio: 0.1
    tier_usd: 15
`
	const badDateYAML = `
plan: test-plan
curated_at: "not-a-date"
models: {}
`
	const malformedYAML = `plan: [this is not: valid yaml`

	tests := []struct {
		name        string
		data        string
		wantErr     bool
		errContains string
	}{
		{"valid table parses", validYAML, false, ""},
		{"bad curated_at date is rejected", badDateYAML, true, "invalid curated_at"},
		{"malformed YAML is rejected", malformedYAML, true, "parsing plan table YAML"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTable([]byte(tt.data))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseTable() error = nil, want error containing %q", tt.errContains)
				}
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("parseTable() error = %q, want it to contain %q", err.Error(), tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseTable() unexpected error: %v", err)
			}
			if got.Plan != "test-plan" {
				t.Errorf("Plan = %q, want %q", got.Plan, "test-plan")
			}
			if got.CuratedAt != "2026-08-01" {
				t.Errorf("CuratedAt = %q, want %q", got.CuratedAt, "2026-08-01")
			}
			mp, ok := got.Models["test-model"]
			if !ok {
				t.Fatal("expected test-model in parsed table")
			}
			if mp.Shape.Input != 100 || mp.Shape.Cached != 1000 || mp.Shape.Output != 50 {
				t.Errorf("Shape = %+v, want {100 1000 50}", mp.Shape)
			}
			if mp.TierUSD != 15 {
				t.Errorf("TierUSD = %v, want 15", mp.TierUSD)
			}
		})
	}
}

// --- Task 3.4/3.5: TestCostPerRequest golden values + D5 fallback ---

func TestCostPerRequest(t *testing.T) {
	table := &Table{
		Plan:      "test",
		CuratedAt: "2026-08-01",
		Models: map[string]ModelPlan{
			"grok-4.5":         {Shape: Shape{Input: 1100, Cached: 71500, Output: 220}, CacheReadRatio: 0.15, TierUSD: 15},
			"hy3":              {Shape: Shape{Input: 830, Cached: 71500, Output: 295}, CacheReadRatio: 0.25, TierUSD: 60, Multiplier: 8},
			"deepseek-v4-pro":  {Shape: Shape{Input: 750, Cached: 82000, Output: 290}, CacheReadRatio: 0.033333, TierUSD: 15},
			"zero-price-model": {Shape: Shape{Input: 100, Cached: 1000, Output: 50}, TierUSD: 15},
		},
	}

	tests := []struct {
		name     string
		ocID     string
		price    Price
		wantCost float64
		wantOK   bool
	}{
		{
			name:     "Grok 4.5 — golden $0.02497",
			ocID:     "grok-4.5",
			price:    Price{InputPerM: 2.00, OutputPerM: 6.00, CacheReadPerM: float64Ptr(0.30)},
			wantCost: 0.02497,
			wantOK:   true,
		},
		{
			name:     "Hy3 — golden $0.0027898",
			ocID:     "hy3",
			price:    Price{InputPerM: 0.14, OutputPerM: 0.58, CacheReadPerM: float64Ptr(0.035)},
			wantCost: 0.0027898,
			wantOK:   true,
		},
		{
			name:     "DeepSeek V4 Pro off-peak — golden $0.0028732",
			ocID:     "deepseek-v4-pro",
			price:    Price{InputPerM: 0.66, OutputPerM: 1.98, CacheReadPerM: float64Ptr(0.022)},
			wantCost: 0.0028732,
			wantOK:   true,
		},
		{
			name:   "untabulated ocID -> ok=false",
			ocID:   "does-not-exist",
			price:  Price{InputPerM: 1, OutputPerM: 1, CacheReadPerM: float64Ptr(1)},
			wantOK: false,
		},
		{
			name:   "zero price throughout -> non-positive cost -> ok=false",
			ocID:   "zero-price-model",
			price:  Price{InputPerM: 0, OutputPerM: 0, CacheReadPerM: float64Ptr(0)},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCost, gotOK := table.CostPerRequest(tt.ocID, tt.price)
			if gotOK != tt.wantOK {
				t.Fatalf("CostPerRequest() ok = %v, want %v", gotOK, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if math.Abs(gotCost-tt.wantCost) > 1e-6 {
				t.Errorf("CostPerRequest() = %v, want %v", gotCost, tt.wantCost)
			}
		})
	}
}

// TestCostPerRequest_CacheReadFallback locks in design D5: a live
// CacheReadPerM is used verbatim when the price source reports one; when it
// is nil, InputPerM*CacheReadRatio is used instead; an explicit 0 is honored
// (never overridden by the ratio).
func TestCostPerRequest_CacheReadFallback(t *testing.T) {
	table := &Table{
		Plan:      "test",
		CuratedAt: "2026-08-01",
		Models: map[string]ModelPlan{
			"model-x": {Shape: Shape{Input: 100, Cached: 1000, Output: 50}, CacheReadRatio: 0.2, TierUSD: 15},
		},
	}

	tests := []struct {
		name          string
		cacheReadPerM *float64
		wantCost      float64
	}{
		{
			name:          "live cache-read price present -> used directly",
			cacheReadPerM: float64Ptr(0.5),
			// cost = (100*1 + 1000*0.5 + 50*2)/1e6 = (100+500+100)/1e6 = 0.0007
			wantCost: 0.0007,
		},
		{
			name:          "nil cache-read price -> input*ratio fallback",
			cacheReadPerM: nil,
			// cacheReadPerM = 1*0.2 = 0.2; cost = (100*1 + 1000*0.2 + 50*2)/1e6 = (100+200+100)/1e6 = 0.0004
			wantCost: 0.0004,
		},
		{
			name:          "explicit 0 cache-read price is honored, not overridden by ratio",
			cacheReadPerM: float64Ptr(0),
			// cost = (100*1 + 1000*0 + 50*2)/1e6 = (100+0+100)/1e6 = 0.0002
			wantCost: 0.0002,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			price := Price{InputPerM: 1, OutputPerM: 2, CacheReadPerM: tt.cacheReadPerM}
			gotCost, ok := table.CostPerRequest("model-x", price)
			if !ok {
				t.Fatal("CostPerRequest() ok = false, want true")
			}
			if math.Abs(gotCost-tt.wantCost) > 1e-9 {
				t.Errorf("CostPerRequest() = %v, want %v", gotCost, tt.wantCost)
			}
		})
	}
}

// --- Task 3.7: TestRequests_AllWindows ---

func TestRequests_AllWindows(t *testing.T) {
	table := &Table{
		Plan:      "test",
		CuratedAt: "2026-08-01",
		Models: map[string]ModelPlan{
			"grok-4.5": {Shape: Shape{Input: 1100, Cached: 71500, Output: 220}, CacheReadRatio: 0.15, TierUSD: 15},
			"hy3":      {Shape: Shape{Input: 830, Cached: 71500, Output: 295}, CacheReadRatio: 0.25, TierUSD: 60, Multiplier: 8},
		},
	}

	t.Run("month == 5h * 5, month == week * 2 (design D1 validated ratios, Grok)", func(t *testing.T) {
		price := Price{InputPerM: 2.00, OutputPerM: 6.00, CacheReadPerM: float64Ptr(0.30)}
		r5h, ok := table.Requests("grok-4.5", price, Window5H)
		if !ok {
			t.Fatal("Requests(Window5H) ok = false")
		}
		rWeek, ok := table.Requests("grok-4.5", price, WindowWeek)
		if !ok {
			t.Fatal("Requests(WindowWeek) ok = false")
		}
		rMonth, ok := table.Requests("grok-4.5", price, WindowMonth)
		if !ok {
			t.Fatal("Requests(WindowMonth) ok = false")
		}
		if math.Abs(rMonth-r5h*5) > 1e-6 {
			t.Errorf("month = %v, want 5h*5 = %v", rMonth, r5h*5)
		}
		if math.Abs(rMonth-rWeek*2) > 1e-6 {
			t.Errorf("month = %v, want week*2 = %v", rMonth, rWeek*2)
		}
	})

	t.Run("Hy3 multiplier 8x — month budget $480, quota ~172058", func(t *testing.T) {
		price := Price{InputPerM: 0.14, OutputPerM: 0.58, CacheReadPerM: float64Ptr(0.035)}
		rMonth, ok := table.Requests("hy3", price, WindowMonth)
		if !ok {
			t.Fatal("Requests(WindowMonth) ok = false")
		}
		if math.Abs(rMonth-172058) > 50 {
			t.Errorf("Hy3 month quota = %v, want ~172058 (budget=$480, 8x multiplier)", rMonth)
		}

		r5hBase, ok := table.Requests("hy3", price, Window5H)
		if !ok {
			t.Fatal("Requests(Window5H) ok = false")
		}
		// 5h quota already carries the 8x multiplier: base (multiplier-1)
		// would be ~4301, so 8x should land near 34400 (OpenCode's published
		// "8x usage" bar chart value).
		if math.Abs(r5hBase-34400) > 100 {
			t.Errorf("Hy3 5h quota (multiplier 8x applied) = %v, want ~34400", r5hBase)
		}
	})
}

// --- Task 3.9: TestReferenceDataFixture ---
//
// Reproduces openspec/changes/live-quota-derivation/reference-data.md's
// published 5h request estimates for all 21 curated models (22 reference
// rows minus MiniMax M2.5, which is deliberately table-absent per design
// D3). Two assertions per row: an exact derived golden (tight tolerance —
// proves the formula/table wiring is stable) and a within-40%-of-published
// check (published numbers are pre-rounded shapes; design's Testing
// Strategy documents 10-36% drift as expected, not a bug).
func TestReferenceDataFixture(t *testing.T) {
	table, err := OpenCodeZenGo()
	if err != nil {
		t.Fatalf("OpenCodeZenGo() error: %v", err)
	}

	tests := []struct {
		ocID          string
		price         Price
		wantDerived5h float64
		published5h   float64
	}{
		{"grok-4.5", Price{2.00, 6.00, float64Ptr(0.30)}, 120.14, 120},
		{"gpt-5.6-luna", Price{0.20, 1.20, float64Ptr(0.02)}, 2049.18, 2050},
		{"glm-5.3", Price{1.40, 4.40, float64Ptr(0.26)}, 197.89, 220},
		{"glm-5.2", Price{1.40, 4.40, float64Ptr(0.26)}, 791.56, 880},
		{"glm-5.1", Price{1.40, 4.40, float64Ptr(0.26)}, 791.56, 880},
		{"kimi-k3", Price{3.00, 15.00, float64Ptr(0.30)}, 98.04, 110},
		{"kimi-k2.7-code", Price{0.95, 4.00, float64Ptr(0.19)}, 993.66, 1350},
		{"kimi-k2.6", Price{0.95, 4.00, float64Ptr(0.16)}, 1150.83, 1150},
		{"mimo-v2.5", Price{0.14, 0.28, float64Ptr(0.0028)}, 30075.19, 30100},
		{"mimo-v2.5-pro", Price{0.435, 0.87, float64Ptr(0.003625)}, 3258.21, 3250},
		{"minimax-m3", Price{0.30, 1.20, float64Ptr(0.06)}, 3207.7, 3200},
		{"minimax-m2.7", Price{0.30, 1.20, float64Ptr(0.06)}, 3389.83, 3400},
		{"muse-spark-1.2-contributor", Price{0.10, 0.20, float64Ptr(0.002)}, 45317.2, 45300},
		{"qwen3.8-max", Price{2.00, 6.00, float64Ptr(0.25)}, 161.81, 160},
		{"qwen3.7-max", Price{2.50, 7.50, float64Ptr(0.50)}, 337.55, 340},
		{"qwen3.7-plus", Price{0.40, 1.60, float64Ptr(0.04)}, 4310.34, 4300},
		{"qwen3.6-plus", Price{0.50, 3.00, float64Ptr(0.05)}, 3269.75, 3300},
		{"deepseek-v4-pro", Price{0.66, 1.98, float64Ptr(0.022)}, 1044.36, 1050},
		{"deepseek-v4-flash", Price{0.22, 0.66, float64Ptr(0.007)}, 7557.6, 7600},
		{"deepseek-v4-flash-vision-exp", Price{0.22, 0.66, float64Ptr(0.007)}, 3778.8, 3800},
		{"hy3", Price{0.14, 0.58, float64Ptr(0.035)}, 34411.07, 34400},
	}

	if len(tests) != 21 {
		t.Fatalf("fixture table has %d rows, want 21 (22 reference-data.md rows minus MiniMax M2.5)", len(tests))
	}

	for _, tt := range tests {
		t.Run(tt.ocID, func(t *testing.T) {
			got, ok := table.Requests(tt.ocID, tt.price, Window5H)
			if !ok {
				t.Fatalf("Requests(%q, Window5H) ok = false, want true", tt.ocID)
			}

			// Exact derived golden — tight tolerance proves the wiring
			// (shape/tier/multiplier from the embedded YAML) is stable.
			if math.Abs(got-tt.wantDerived5h) > 0.5 {
				t.Errorf("%s: derived 5h = %v, want golden %v", tt.ocID, got, tt.wantDerived5h)
			}

			// Within 40% of OpenCode's published estimate (design's Testing
			// Strategy: shape-rounding drift is expected, not a bug).
			delta := math.Abs(got-tt.published5h) / tt.published5h
			if delta > 0.40 {
				t.Errorf("%s: derived 5h = %v is %.1f%% off published %v, want within 40%%", tt.ocID, got, delta*100, tt.published5h)
			}
		})
	}

	// MiniMax M2.5 is deliberately table-absent (design D3): resolvable
	// price but no published shape to curate. Locks the fallback path stays
	// reachable rather than silently inheriting M2.7's shape.
	t.Run("minimax-m2.5 is table-absent (D3 — no fabricated shape)", func(t *testing.T) {
		_, ok := table.Requests("minimax-m2.5", Price{InputPerM: 0.30, OutputPerM: 1.20, CacheReadPerM: float64Ptr(0.06)}, Window5H)
		if ok {
			t.Error("minimax-m2.5 unexpectedly resolved via the table — expected table-absent per design D3")
		}
	})

	// Ox Alpha Free: no shape, no price, excluded entirely.
	t.Run("ox-alpha-free is table-absent (no shape, no price)", func(t *testing.T) {
		_, ok := table.Requests("ox-alpha-free", Price{}, Window5H)
		if ok {
			t.Error("ox-alpha-free unexpectedly resolved via the table")
		}
	})
}
