package profile

import (
	"strings"
	"testing"
)

func TestLoad_ValidMultiMetricProfile(t *testing.T) {
	data := []byte(`
version: 1
roles:
  - id: custom-role
    weights:
      intelligence: 0.6
      coding: 0.4
    min_context: 100000
    requires: ["reasoning"]
`)

	p, err := Load(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.Roles) != 1 {
		t.Fatalf("expected 1 role, got %d", len(p.Roles))
	}
	r := p.Roles[0]
	if r.Weights[MetricIntelligence] != 0.6 || r.Weights[MetricCoding] != 0.4 {
		t.Errorf("unexpected weights: %+v", r.Weights)
	}
	if r.MinContext == nil || *r.MinContext != 100000 {
		t.Errorf("expected min_context 100000, got %v", r.MinContext)
	}
	if len(r.Requires) != 1 || r.Requires[0] != "reasoning" {
		t.Errorf("expected requires [reasoning], got %v", r.Requires)
	}
}

func TestLoad_UnknownMetricKeyRejected(t *testing.T) {
	data := []byte(`
version: 1
roles:
  - id: custom-role
    weights:
      creativity: 1.0
`)

	_, err := Load(data)
	if err == nil {
		t.Fatal("expected error for unknown metric key, got nil")
	}
	if !strings.Contains(err.Error(), "creativity") {
		t.Errorf("expected error to name the unknown key %q, got: %v", "creativity", err)
	}
}

func TestLoad_MissingIDFailsValidation(t *testing.T) {
	data := []byte(`
version: 1
roles:
  - description: "role with no id"
    weights:
      intelligence: 1.0
`)

	_, err := Load(data)
	if err == nil {
		t.Fatal("expected error for missing id, got nil")
	}
	if !strings.Contains(err.Error(), "id") {
		t.Errorf("expected error to mention missing id, got: %v", err)
	}
}

func TestLoad_MalformedYAMLFails(t *testing.T) {
	data := []byte("version: 1\nroles: [this is not: valid: yaml")

	_, err := Load(data)
	if err == nil {
		t.Fatal("expected error for malformed YAML, got nil")
	}
}

func TestLoad_DefaultsMerge(t *testing.T) {
	minCtx := int64(8000)
	data := []byte(`
version: 1
defaults:
  min_context: 8000
roles:
  - id: inherits-default
    weights:
      intelligence: 1.0
  - id: overrides-default
    min_context: 32000
    weights:
      intelligence: 1.0
  - id: no-weights
    min_context: 8000
`)

	p, err := Load(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	byID := make(map[string]Role, len(p.Roles))
	for _, r := range p.Roles {
		byID[r.ID] = r
	}

	t.Run("Role inherits default hard constraints", func(t *testing.T) {
		r := byID["inherits-default"]
		if r.MinContext == nil || *r.MinContext != minCtx {
			t.Errorf("expected inherited min_context %d, got %v", minCtx, r.MinContext)
		}
	})

	t.Run("Role overrides a default", func(t *testing.T) {
		r := byID["overrides-default"]
		if r.MinContext == nil || *r.MinContext != 32000 {
			t.Errorf("expected overridden min_context 32000, got %v", r.MinContext)
		}
	})

	t.Run("Weights are never defaulted", func(t *testing.T) {
		r := byID["no-weights"]
		if len(r.Weights) != 0 {
			t.Errorf("expected empty weights, got %+v", r.Weights)
		}
	})
}
