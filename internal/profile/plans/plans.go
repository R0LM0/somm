// Package plans embeds the curated OpenCode plan quota table consumed by
// the "quota" ranking currency (plan-quota-currency spec, Requirement:
// Embedded Quota Table). It mirrors the embed pattern used by
// internal/profile's gentle-ai preset: no network or filesystem access
// outside the compiled binary.
package plans

import (
	_ "embed"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

//go:embed opencode-zen-go.yaml
var openCodeZenGoData []byte

// dateLayout is the required format for Table.MeasuredAt, validated at load
// time (plan-quota-currency spec, Requirement: Embedded Quota Table).
const dateLayout = "2006-01-02"

// Table is an embedded OpenCode plan quota snapshot: which models are
// known, how many requests/5h each affords, and when that snapshot was
// measured.
type Table struct {
	Plan       string         `yaml:"plan"`
	MeasuredAt string         `yaml:"measured_at"`
	Models     map[string]int `yaml:"models"`
}

// OpenCodeZenGo loads and validates the embedded OpenCode Zen Go plan quota
// table. It fails loud if the embedded YAML is malformed or MeasuredAt does
// not parse as 2006-01-02.
func OpenCodeZenGo() (*Table, error) {
	t, err := parseTable(openCodeZenGoData)
	if err != nil {
		return nil, fmt.Errorf("loading embedded opencode-zen-go plan table: %w", err)
	}
	return t, nil
}

// parseTable unmarshals and validates raw plan quota table YAML.
func parseTable(data []byte) (*Table, error) {
	var t Table
	if err := yaml.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("parsing plan table YAML: %w", err)
	}
	if _, err := time.Parse(dateLayout, t.MeasuredAt); err != nil {
		return nil, fmt.Errorf("invalid measured_at %q (want %s): %w", t.MeasuredAt, dateLayout, err)
	}
	return &t, nil
}

// Requests returns the requests_per_5h quota for ocID. ok is false when
// ocID is absent from the table — never a 0-as-default (plan-quota-currency
// spec, Requirement: Embedded Quota Table, "Table omits an unknown model
// id").
func (t *Table) Requests(ocID string) (int, bool) {
	v, ok := t.Models[ocID]
	return v, ok
}
