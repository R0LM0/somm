package profile

import "testing"

func TestPreset_LoadsWith19Roles(t *testing.T) {
	p, err := Preset()
	if err != nil {
		t.Fatalf("unexpected error loading embedded preset: %v", err)
	}
	if len(p.Roles) != 19 {
		t.Fatalf("expected 19 roles in gentle-ai preset, got %d", len(p.Roles))
	}
}

func TestPreset_WeightMapping(t *testing.T) {
	p, err := Preset()
	if err != nil {
		t.Fatalf("unexpected error loading embedded preset: %v", err)
	}

	wantWeights := map[string]map[string]float64{
		"orchestrator":       {MetricIntelligence: 1.0},
		"sdd-init":           {},
		"sdd-onboard":        {MetricIntelligence: 1.0},
		"sdd-explore":        {MetricAgentic: 1.0},
		"sdd-propose":        {MetricIntelligence: 1.0},
		"sdd-spec":           {MetricIntelligence: 1.0},
		"sdd-design":         {MetricIntelligence: 1.0},
		"sdd-tasks":          {},
		"sdd-apply":          {MetricCoding: 1.0},
		"sdd-verify":         {MetricIntelligence: 1.0},
		"sdd-archive":        {},
		"review-risk":        {MetricIntelligence: 1.0},
		"review-readability": {MetricIntelligence: 1.0},
		"review-reliability": {MetricIntelligence: 1.0},
		"review-resilience":  {MetricIntelligence: 1.0},
		"review-refuter":     {MetricIntelligence: 1.0},
		"jd-judge-a":         {MetricIntelligence: 1.0},
		"jd-judge-b":         {MetricIntelligence: 1.0},
		"jd-fix-agent":       {MetricCoding: 1.0},
	}

	seen := make(map[string]bool, len(p.Roles))
	for _, r := range p.Roles {
		seen[r.ID] = true
		want, ok := wantWeights[r.ID]
		if !ok {
			t.Errorf("unexpected role id %q in preset", r.ID)
			continue
		}
		if len(r.Weights) != len(want) {
			t.Errorf("role %q: expected %d weight(s), got %d (%+v)", r.ID, len(want), len(r.Weights), r.Weights)
			continue
		}
		for k, v := range want {
			if r.Weights[k] != v {
				t.Errorf("role %q: expected weight %s=%v, got %v", r.ID, k, v, r.Weights[k])
			}
		}
		if r.Criticidad == "" {
			t.Errorf("role %q: expected non-empty criticidad", r.ID)
		}
		if r.Description == "" {
			t.Errorf("role %q: expected non-empty description", r.ID)
		}
	}

	for id := range wantWeights {
		if !seen[id] {
			t.Errorf("expected role %q in preset, not found", id)
		}
	}
}
