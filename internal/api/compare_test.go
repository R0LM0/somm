package api

import (
	"testing"
)

func TestExtractComparison(t *testing.T) {
	d1 := 85.5
	d2 := 72.3
	d3 := 60.1
	ctx := int64(200000)
	orModels := []ORModel{
		{
			ID:   "deepseek/deepseek-v4-pro",
			Name: "DeepSeek V4 Pro",
			Pricing: &Pricing{
				Prompt:     "0.000435",
				Completion: "0.00087",
			},
			ContextLength: &ctx,
			Benchmarks: &Benchmarks{
				ArtificialAnalysis: &ArtificialAnalysis{
					IntelligenceIndex: &d1,
					CodingIndex:       &d2,
					AgenticIndex:      &d3,
				},
			},
			Reasoning: &Reasoning{
				SupportedEfforts: []string{"xhigh", "high"},
			},
		},
		{
			ID:   "moonshotai/kimi-k3",
			Name: "Kimi K3",
		},
	}

	t.Run("found with full data", func(t *testing.T) {
		mc, err := extractComparison("deepseek/deepseek-v4-pro", orModels)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mc.Name != "DeepSeek V4 Pro" {
			t.Errorf("name = %q, want %q", mc.Name, "DeepSeek V4 Pro")
		}
		if mc.Intelligence == nil || *mc.Intelligence != 85.5 {
			t.Errorf("intelligence = %v, want 85.5", mc.Intelligence)
		}
		if mc.Coding == nil || *mc.Coding != 72.3 {
			t.Errorf("coding = %v, want 72.3", mc.Coding)
		}
		if mc.Agentic == nil || *mc.Agentic != 60.1 {
			t.Errorf("agentic = %v, want 60.1", mc.Agentic)
		}
		if mc.PriceInput == nil || *mc.PriceInput != 435.0 {
			t.Errorf("price_input = %v, want 435.0", mc.PriceInput)
		}
		if mc.PriceOutput == nil || *mc.PriceOutput != 870.0 {
			t.Errorf("price_output = %v, want 870.0", mc.PriceOutput)
		}
		if mc.ContextLength == nil || *mc.ContextLength != 200000 {
			t.Errorf("context_length = %v, want 200000", mc.ContextLength)
		}
		if len(mc.Reasoning) != 2 || mc.Reasoning[0] != "xhigh" {
			t.Errorf("reasoning = %v, want [xhigh high]", mc.Reasoning)
		}
	})

	t.Run("found minimal data", func(t *testing.T) {
		mc, err := extractComparison("moonshotai/kimi-k3", orModels)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mc.Name != "Kimi K3" {
			t.Errorf("name = %q, want %q", mc.Name, "Kimi K3")
		}
		if mc.Intelligence != nil {
			t.Errorf("intelligence should be nil, got %v", mc.Intelligence)
		}
		if mc.PriceInput != nil {
			t.Errorf("price_input should be nil, got %v", mc.PriceInput)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := extractComparison("nonexistent/model", orModels)
		if err == nil {
			t.Fatal("expected error for nonexistent model")
		}
	})

	t.Run("case insensitive", func(t *testing.T) {
		mc, err := extractComparison("DeepSeek/DeepSeek-V4-Pro", orModels)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mc.Name != "DeepSeek V4 Pro" {
			t.Errorf("name = %q, want %q", mc.Name, "DeepSeek V4 Pro")
		}
	})
}

func TestDetermineWinners(t *testing.T) {
	d1 := 85.0
	d2 := 90.0
	c1 := 72.0
	c2 := 88.0
	a1 := 55.0
	a2 := 65.0
	p1 := 0.5
	p2 := 1.2
	o1 := 1.0
	o2 := 2.4

	comparisons := []ModelComparison{
		{
			ID:           "model-a",
			Name:         "Model A",
			Intelligence: &d1,
			Coding:       &c1,
			Agentic:      &a1,
			PriceInput:   &p1,
			PriceOutput:  &o1,
		},
		{
			ID:           "model-b",
			Name:         "Model B",
			Intelligence: &d2,
			Coding:       &c2,
			Agentic:      &a2,
			PriceInput:   &p2,
			PriceOutput:  &o2,
		},
	}

	winners := determineWinners(comparisons)

	if v, ok := winners["best_coding"]; !ok {
		t.Error("missing best_coding winner")
	} else if v != "Model B (88.0)" {
		t.Errorf("best_coding = %q, want %q", v, "Model B (88.0)")
	}

	if v, ok := winners["best_price"]; !ok {
		t.Error("missing best_price winner")
	} else if v != "Model A ($0.500/M)" {
		t.Errorf("best_price = %q, want %q", v, "Model A ($0.500/M)")
	}

	if v, ok := winners["best_intelligence"]; !ok {
		t.Error("missing best_intelligence winner")
	} else if v != "Model B (90.0)" {
		t.Errorf("best_intelligence = %q, want %q", v, "Model B (90.0)")
	}

	if v, ok := winners["best_agentic"]; !ok {
		t.Error("missing best_agentic winner")
	} else if v != "Model B (65.0)" {
		t.Errorf("best_agentic = %q, want %q", v, "Model B (65.0)")
	}

	if v, ok := winners["best_balance"]; !ok {
		t.Error("missing best_balance winner")
	} else if v != "Model A (72.0 coding / $0.500 = 144.0 points/$)" {
		t.Errorf("best_balance = %q, want %q", v, "Model A (72.0 coding / $0.500 = 144.0 points/$)")
	}
}

func TestFindHighestNilValues(t *testing.T) {
	v1 := 50.0
	comparisons := []ModelComparison{
		{ID: "a", Name: "A"},
		{ID: "b", Name: "B", Coding: &v1},
	}

	best := findHighest(comparisons, func(mc ModelComparison) *float64 {
		return mc.Coding
	})
	if best == nil || best.ID != "b" {
		t.Errorf("expected model b, got %v", best)
	}
}

func TestFindLowestNilValues(t *testing.T) {
	v1 := 50.0
	comparisons := []ModelComparison{
		{ID: "a", Name: "A"},
		{ID: "b", Name: "B", PriceInput: &v1},
	}

	best := findLowest(comparisons, func(mc ModelComparison) *float64 {
		return mc.PriceInput
	})
	if best == nil || best.ID != "b" {
		t.Errorf("expected model b, got %v", best)
	}
}

func TestFormatComparison(t *testing.T) {
	v1 := 85.0
	v2 := 92.0
	c1 := 70.0
	c2 := 88.0
	p1 := 0.5
	p2 := 1.2
	ctx1 := int64(200000)
	ctx2 := int64(128000)

	result := &ComparisonResult{
		Models: []ModelComparison{
			{
				ID:            "deepseek/deepseek-v4-pro",
				Name:          "DeepSeek V4 Pro",
				Intelligence:  &v1,
				Coding:        &c1,
				PriceInput:    &p1,
				ContextLength: &ctx1,
				Reasoning:     []string{"xhigh", "high"},
			},
			{
				ID:            "openai/gpt-5",
				Name:          "GPT-5",
				Intelligence:  &v2,
				Coding:        &c2,
				PriceInput:    &p2,
				ContextLength: &ctx2,
			},
		},
		Winners: map[string]string{
			"best_coding":   "GPT-5 (88.0)",
			"best_price":    "DeepSeek V4 Pro ($0.500/M)",
			"best_balance":  "DeepSeek V4 Pro (70.0 coding / $0.500 = 140.0 points/$)",
			"best_intelligence": "GPT-5 (92.0)",
		},
	}

	text := FormatComparison(result)

	if !contains(text, "Comparacion: DeepSeek V4 Pro vs GPT-5") {
		t.Errorf("missing comparison header, got:\n%s", text)
	}
	if !contains(text, "| Intelligence |") {
		t.Error("missing Intelligence row")
	}
	if !contains(text, "| Coding |") {
		t.Error("missing Coding row")
	}
	if !contains(text, "| Reasoning |") {
		t.Error("missing Reasoning row")
	}
	if !contains(text, "xhigh, high") {
		t.Errorf("missing reasoning values, got:\n%s", text)
	}
	if !contains(text, "Mejor para coding: GPT-5") {
		t.Errorf("missing coding winner, got:\n%s", text)
	}
	if !contains(text, "Mejor para price: DeepSeek V4 Pro") {
		t.Errorf("missing price winner, got:\n%s", text)
	}
	if !contains(text, "Mejor balance: DeepSeek V4 Pro") {
		t.Errorf("missing balance winner, got:\n%s", text)
	}
}


