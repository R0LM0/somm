package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// DiscoveredModel is one model reported by the local opencode CLI. Prices are
// USD per 1M tokens (design D7), matching internal/profile/plans.Price's
// *PerM convention — not api.Money's per-token convention.
type DiscoveredModel struct {
	ProviderID    string
	ID            string
	Name          string
	InputPerM     float64
	OutputPerM    float64
	CacheReadPerM *float64
	ContextLength *int64
}

// ProviderDiscoverer reports the models served by providers configured on
// this host (default implementation: the local opencode CLI, design D1/D2).
// Every error is non-fatal: callers warn and continue with the existing
// catalog (design D4).
type ProviderDiscoverer interface {
	Discover(ctx context.Context) ([]DiscoveredModel, error)
}

// discoverCostCache is the "cost.cache" object reported per model:
// cache-read/write prices, USD per 1M tokens.
type discoverCostCache struct {
	Read  float64 `json:"read"`
	Write float64 `json:"write"`
}

// discoverCost is the "cost" object reported per model. A null cost is a
// per-entry drop signal (design D4) — usage-based pricing is unavailable for
// that model.
type discoverCost struct {
	Input  float64            `json:"input"`
	Output float64            `json:"output"`
	Cache  *discoverCostCache `json:"cache,omitempty"`
}

// discoverLimit is the "limit" object reported per model; only Context is
// consumed today.
type discoverLimit struct {
	Context *int64 `json:"context,omitempty"`
}

// discoverRecord is the raw per-model JSON shape emitted by
// `opencode models --verbose` (design D1). Unknown extra fields decode and
// are silently ignored — this is encoding/json's default behavior; no
// DisallowUnknownFields call is made (threat matrix: PATH hijack).
type discoverRecord struct {
	ID         string         `json:"id"`
	ProviderID string         `json:"providerID"`
	Name       string         `json:"name"`
	Cost       *discoverCost  `json:"cost"`
	Limit      *discoverLimit `json:"limit,omitempty"`
}

// parseDiscoverOutput decodes the raw stdout of `opencode models --verbose`
// into []DiscoveredModel.
//
// The real output is NOT a JSON array or an NDJSON document (verified
// against a live `opencode models --verbose` capture): it is a
// `providerID/id` label line followed by a pretty-printed JSON object,
// repeated per model, with no enclosing envelope at all. splitDiscoverBlocks
// extracts each top-level object, discarding the label lines between them.
//
// An input that ends mid-object (truncated stdout — depth never returns to
// zero) returns an error; the caller (execDiscoverer, Phase 3) treats it as
// non-fatal and continues with the existing catalog (design D4). Otherwise,
// a per-block malformed record — invalid JSON body, missing id/providerID,
// or a null cost — is dropped individually; it never fails the whole parse
// (design D4's one exception to the uniform failure taxonomy). Input with no
// recognizable object at all (e.g. an unexpected CLI output format) yields
// an empty result, not an error — there is no envelope to validate against.
func parseDiscoverOutput(raw []byte) ([]DiscoveredModel, error) {
	blocks, truncated := splitDiscoverBlocks(raw)
	if truncated {
		return nil, fmt.Errorf("parsing discover output: truncated JSON object (stdout cut off mid-record)")
	}

	models := make([]DiscoveredModel, 0, len(blocks))
	for _, block := range blocks {
		var rec discoverRecord
		if err := json.Unmarshal(block, &rec); err != nil {
			continue // per-entry drop: malformed object body (design D4)
		}
		if strings.TrimSpace(rec.ID) == "" || strings.TrimSpace(rec.ProviderID) == "" {
			continue
		}
		if rec.Cost == nil {
			continue
		}

		dm := DiscoveredModel{
			ProviderID: rec.ProviderID,
			ID:         rec.ID,
			Name:       rec.Name,
			InputPerM:  rec.Cost.Input,
			OutputPerM: rec.Cost.Output,
		}
		if rec.Cost.Cache != nil {
			read := rec.Cost.Cache.Read
			dm.CacheReadPerM = &read
		}
		if rec.Limit != nil && rec.Limit.Context != nil {
			ctxLen := *rec.Limit.Context
			dm.ContextLength = &ctxLen
		}
		models = append(models, dm)
	}
	return models, nil
}

// splitDiscoverBlocks extracts each top-level JSON object from raw stdout
// that interleaves non-JSON label lines with JSON objects (single-line or
// pretty-printed). A line is treated as the start of an object once it
// begins (after trimming) with `{`; brace-depth counting per line — not a
// full JSON tokenizer — determines where that object ends. This is
// sufficient because ids/names/providerIDs in this schema never contain
// literal `{`/`}` characters (a hostile field containing one would only
// mis-split that one block, which then fails to unmarshal and is dropped by
// the per-entry gate above, not silently misread).
//
// truncated is true when the input ends with an object that never returned
// to brace-depth zero (stdout cut off mid-record) — the one case that must
// surface as an error rather than degrade silently (design D4).
func splitDiscoverBlocks(raw []byte) (blocks [][]byte, truncated bool) {
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64*1024), 8<<20) // 8 MiB guard (design threat matrix)

	var buf bytes.Buffer
	depth := 0
	inObject := false
	for scanner.Scan() {
		line := scanner.Text()
		if !inObject {
			if !strings.HasPrefix(strings.TrimSpace(line), "{") {
				continue // label line or blank — not the start of an object
			}
			inObject = true
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
		depth += strings.Count(line, "{") - strings.Count(line, "}")
		if depth <= 0 {
			blocks = append(blocks, append([]byte(nil), buf.Bytes()...))
			buf.Reset()
			inObject = false
			depth = 0
		}
	}
	return blocks, inObject
}
