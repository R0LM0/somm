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

func TestLoad_UnknownObjectiveValueRejected(t *testing.T) {
	data := []byte(`
version: 1
selection:
  objective: cheapest
roles:
  - id: custom-role
`)

	_, err := Load(data)
	if err == nil {
		t.Fatal("expected error for unknown objective value, got nil")
	}
	if !strings.Contains(err.Error(), "cheapest") {
		t.Errorf("expected error to name the unknown objective %q, got: %v", "cheapest", err)
	}
}

func TestLoad_UnknownCurrencyValueRejected(t *testing.T) {
	data := []byte(`
version: 1
roles:
  - id: custom-role
    selection:
      currency: euros
`)

	_, err := Load(data)
	if err == nil {
		t.Fatal("expected error for unknown currency value, got nil")
	}
	if !strings.Contains(err.Error(), "euros") {
		t.Errorf("expected error to name the unknown currency %q, got: %v", "euros", err)
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

func TestLoad_DefaultSelectionAppliesToEveryRole(t *testing.T) {
	data := []byte(`
version: 1
roles:
  - id: role-a
    weights:
      intelligence: 1.0
  - id: role-b
`)

	p, err := Load(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.Roles) != 2 {
		t.Fatalf("expected 2 roles, got %d", len(p.Roles))
	}
	for _, r := range p.Roles {
		if r.Selection == nil {
			t.Fatalf("role %q: expected non-nil resolved Selection", r.ID)
		}
		if r.Selection.Objective != "value" {
			t.Errorf("role %q: effective objective = %q, want %q", r.ID, r.Selection.Objective, "value")
		}
		if r.Selection.Currency != "usd" {
			t.Errorf("role %q: effective currency = %q, want %q", r.ID, r.Selection.Currency, "usd")
		}
	}
}

func TestLoad_PerRoleSelectionOverride(t *testing.T) {
	t.Run("role overrides only objective, inherits profile currency", func(t *testing.T) {
		data := []byte(`
version: 1
selection:
  objective: value
  currency: quota
roles:
  - id: role-a
    selection:
      objective: quality
`)
		p, err := Load(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		r := p.Roles[0]
		if r.Selection.Objective != "quality" {
			t.Errorf("effective objective = %q, want %q", r.Selection.Objective, "quality")
		}
		if r.Selection.Currency != "quota" {
			t.Errorf("effective currency = %q, want %q (inherited)", r.Selection.Currency, "quota")
		}
	})

	t.Run("role overrides only currency, inherits profile objective", func(t *testing.T) {
		data := []byte(`
version: 1
selection:
  objective: quality
roles:
  - id: role-a
    selection:
      currency: quota
`)
		p, err := Load(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		r := p.Roles[0]
		if r.Selection.Objective != "quality" {
			t.Errorf("effective objective = %q, want %q (inherited)", r.Selection.Objective, "quality")
		}
		if r.Selection.Currency != "quota" {
			t.Errorf("effective currency = %q, want %q", r.Selection.Currency, "quota")
		}
	})

	t.Run("role selection absent, profile-level selection applies fully", func(t *testing.T) {
		data := []byte(`
version: 1
selection:
  objective: budget
  currency: usd
defaults:
  max_input_price: 5.0
roles:
  - id: role-a
`)
		p, err := Load(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		r := p.Roles[0]
		if r.Selection.Objective != "budget" {
			t.Errorf("effective objective = %q, want %q", r.Selection.Objective, "budget")
		}
		if r.Selection.Currency != "usd" {
			t.Errorf("effective currency = %q, want %q", r.Selection.Currency, "usd")
		}
	})
}

func TestLoad_CurrencyExplicitTracksWhereCurrencyWasSet(t *testing.T) {
	t.Run("unset anywhere: not explicit", func(t *testing.T) {
		data := []byte(`
version: 1
roles:
  - id: role-a
`)
		p, err := Load(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Roles[0].currencyExplicit {
			t.Error("expected currencyExplicit=false when currency was never set")
		}
	})

	t.Run("set at profile level: explicit", func(t *testing.T) {
		data := []byte(`
version: 1
selection:
  currency: quota
roles:
  - id: role-a
`)
		p, err := Load(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !p.Roles[0].currencyExplicit {
			t.Error("expected currencyExplicit=true when profile-level currency was set")
		}
	})

	t.Run("set at role level: explicit", func(t *testing.T) {
		data := []byte(`
version: 1
roles:
  - id: role-a
    selection:
      currency: quota
`)
		p, err := Load(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !p.Roles[0].currencyExplicit {
			t.Error("expected currencyExplicit=true when role-level currency was set")
		}
	})
}

func TestLoad_BudgetObjectiveRequiresEffectiveCeiling(t *testing.T) {
	t.Run("no max_input_price anywhere fails, naming the missing ceiling", func(t *testing.T) {
		data := []byte(`
version: 1
roles:
  - id: role-a
    selection:
      objective: budget
`)
		_, err := Load(data)
		if err == nil {
			t.Fatal("expected error for budget objective with no effective max_input_price, got nil")
		}
		if !strings.Contains(err.Error(), "max_input_price") {
			t.Errorf("expected error to name the missing max_input_price ceiling, got: %v", err)
		}
		if !strings.Contains(err.Error(), "role-a") {
			t.Errorf("expected error to name the role, got: %v", err)
		}
	})

	t.Run("defaults-inherited ceiling loads successfully", func(t *testing.T) {
		data := []byte(`
version: 1
defaults:
  max_input_price: 5.0
roles:
  - id: role-a
    selection:
      objective: budget
`)
		p, err := Load(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		r := p.Roles[0]
		if r.MaxInputPrice == nil || *r.MaxInputPrice != 5.0 {
			t.Errorf("expected inherited max_input_price 5.0, got %v", r.MaxInputPrice)
		}
	})
}

func TestLoad_UnknownFrequencyValueRejected(t *testing.T) {
	data := []byte(`
version: 1
roles:
  - id: custom-role
    frequency: extreme
`)

	_, err := Load(data)
	if err == nil {
		t.Fatal("expected error for unknown frequency value, got nil")
	}
	if !strings.Contains(err.Error(), "extreme") {
		t.Errorf("expected error to name the unknown frequency %q, got: %v", "extreme", err)
	}
}

func TestLoad_FrequencyAcceptedValuesLoad(t *testing.T) {
	for _, freq := range []string{"high", "medium", "low"} {
		t.Run(freq, func(t *testing.T) {
			data := []byte(`
version: 1
roles:
  - id: custom-role
    frequency: ` + freq + `
`)
			p, err := Load(data)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if p.Roles[0].Frequency != freq {
				t.Errorf("Frequency = %q, want %q", p.Roles[0].Frequency, freq)
			}
		})
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
