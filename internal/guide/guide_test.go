package guide_test

import (
	"strings"
	"testing"

	"github.com/R0LM0/somm/v2/internal/guide"
)

func TestExtract(t *testing.T) {
	full, err := guide.Extract("")
	if err != nil {
		t.Fatalf("empty agent: unexpected error: %v", err)
	}
	if !strings.Contains(full, "Guía Gentle AI") {
		t.Errorf("empty agent: expected full guide content, got truncated content")
	}

	tests := []struct {
		name    string
		agent   string
		want    []string
		notWant []string
		wantErr string
	}{
		{
			name:  "exact sdd-orchestrator",
			agent: "sdd-orchestrator",
			want:  []string{"### sdd-orchestrator", "Sistema nervioso del pipeline", "CRÍTICO"},
		},
		{
			name:  "case-insensitive SDD-INIT",
			agent: "SDD-INIT",
			want:  []string{"### sdd-init", "Ingestión de contexto inicial"},
		},
		{
			name:    "prefix match orchestrator",
			agent:   "orchestrator",
			want:    []string{"### sdd-orchestrator", "ROUTING, DELEGACIÓN, COORDINACIÓN"},
			notWant: []string{"### sdd-init"},
		},
		{
			name:  "review prefix risk",
			agent: "risk",
			want:  []string{"### review-risk", "R1 — Risk Reviewer"},
		},
		{
			name:  "jd prefix judge-a",
			agent: "judge-a",
			want:  []string{"### jd-judge-a", "Blind Judges"},
		},
		{
			name:    "heading boundary sdd-onboard",
			agent:   "sdd-onboard",
			want:    []string{"### sdd-onboard"},
			notWant: []string{"### sdd-explore"},
		},
		{
			name:    "not found",
			agent:   "unknown-agent",
			wantErr: `Agent "unknown-agent" not found in guia_gentle_ai.md. Available agents:`,
		},
		{
			name:    "not found available list includes known agents",
			agent:   "missing",
			wantErr: "sdd-orchestrator",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := guide.Extract(tt.agent)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("expected result to contain %q, got:\n%s", w, got)
				}
			}
			for _, w := range tt.notWant {
				if strings.Contains(got, w) {
					t.Errorf("expected result NOT to contain %q, got:\n%s", w, got)
				}
			}
		})
	}
}
