// Package plans embeds the curated OpenCode plan quota table consumed by
// the "quota" ranking currency (plan-quota-currency spec, Requirement:
// Embedded Quota Table). It mirrors the embed pattern used by
// internal/profile's gentle-ai preset: no network or filesystem access
// outside the compiled binary.
//
// The table stores curated *shape + tier + multiplier*, never a final
// requests-per-window count: that number is derived at request time from
// these curated inputs plus the model's live price (design D1). This
// package never imports internal/api; Price is the seam a caller projects
// live pricing through.
package plans

import (
	_ "embed"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

//go:embed opencode-zen-go.yaml
var openCodeZenGoData []byte

// dateLayout is the required format for Table.CuratedAt, validated at load
// time (plan-quota-currency spec, Requirement: Embedded Quota Table).
const dateLayout = "2006-01-02"

// Shape is a curated per-request token pattern: input/cached/output token
// counts (plan-quota-currency spec, Requirement: Embedded Quota Table).
type Shape struct {
	Input  float64 `yaml:"input"`
	Cached float64 `yaml:"cached"`
	Output float64 `yaml:"output"`
}

// ModelPlan is one curated model's derivation inputs: token shape, the
// cache-read discount ratio (cache-read price ÷ input price, design D5),
// the included-usage tier in USD, and an optional multiplier (0 means 1).
type ModelPlan struct {
	Shape          Shape   `yaml:"shape"`
	CacheReadRatio float64 `yaml:"cache_read_ratio"`
	TierUSD        float64 `yaml:"tier_usd"`
	Multiplier     float64 `yaml:"multiplier"` // 0 => 1
}

// Table is an embedded OpenCode plan quota snapshot: which models are
// known, their curated shape/tier/multiplier, and when that curation
// snapshot was taken.
type Table struct {
	Plan      string               `yaml:"plan"`
	CuratedAt string               `yaml:"curated_at"`
	Models    map[string]ModelPlan `yaml:"models"`
}

// Price is live per-1M-token pricing, projected by the caller (internal/api)
// from a model's resolved market price. CacheReadPerM nil means the price
// source did not report a cache-read price for this model; a ModelPlan's
// CacheReadRatio is used as a fallback in that case (design D5).
type Price struct {
	InputPerM     float64
	OutputPerM    float64
	CacheReadPerM *float64
}

// Window selects which included-usage window budget divides TierUSD.
type Window int

const (
	Window5H Window = iota
	WindowWeek
	WindowMonth
)

// windowDivisor: budget_usd = tier_usd / divisor. Design D1: 5/2/1 are
// exact Go constants, not curated per-model values — undoing published
// display rounding, month/5h == 5.0 and month/week == 2.0 hold for the
// large majority of curated models.
var windowDivisor = map[Window]float64{
	Window5H:    5,
	WindowWeek:  2,
	WindowMonth: 1,
}

// OpenCodeZenGo loads and validates the embedded OpenCode Zen Go plan quota
// table. It fails loud if the embedded YAML is malformed or CuratedAt does
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
	if _, err := time.Parse(dateLayout, t.CuratedAt); err != nil {
		return nil, fmt.Errorf("invalid curated_at %q (want %s): %w", t.CuratedAt, dateLayout, err)
	}
	return &t, nil
}

// CostPerRequest derives the cost of a single request for ocID under the
// live price p:
//
//	cost = (shape.Input*P_in + shape.Cached*P_cacheRead + shape.Output*P_out) / 1e6
//
// P_cacheRead is p.CacheReadPerM when the price source reported one, else
// p.InputPerM * ModelPlan.CacheReadRatio (design D5). ok is false when ocID
// is absent from the table, or the derived cost is not positive.
func (t *Table) CostPerRequest(ocID string, p Price) (float64, bool) {
	mp, ok := t.Models[ocID]
	if !ok {
		return 0, false
	}

	cacheReadPerM := mp.CacheReadRatio * p.InputPerM
	if p.CacheReadPerM != nil {
		cacheReadPerM = *p.CacheReadPerM
	}

	cost := (mp.Shape.Input*p.InputPerM + mp.Shape.Cached*cacheReadPerM + mp.Shape.Output*p.OutputPerM) / 1e6
	if cost <= 0 {
		return 0, false
	}
	return cost, true
}

// Requests derives the number of requests affordable in window w for ocID
// under the live price p:
//
//	requests(w) = multiplier * (tier_usd / windowDivisor[w]) / cost_per_request
//
// multiplier defaults to 1 when ModelPlan.Multiplier is 0 (design D1/Hy3).
// ok is false when ocID is untabulated or the derived cost is not positive
// (plan-quota-currency spec, Requirement: Embedded Quota Table, "Table omits
// an unknown model id" — never a 0-as-default).
func (t *Table) Requests(ocID string, p Price, w Window) (float64, bool) {
	mp, ok := t.Models[ocID]
	if !ok {
		return 0, false
	}

	cost, ok := t.CostPerRequest(ocID, p)
	if !ok {
		return 0, false
	}

	multiplier := mp.Multiplier
	if multiplier == 0 {
		multiplier = 1
	}

	budget := mp.TierUSD / windowDivisor[w]
	return multiplier * budget / cost, true
}
