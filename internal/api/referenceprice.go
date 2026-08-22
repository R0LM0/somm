package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// ReferencePricer looks up models.dev-style list pricing (USD per 1M
// tokens, D7 *PerM convention) for a provider+model pair, independent of how
// this host is actually authenticated (design: reference pricing for
// flat-rate/OAuth providers). A Lookup result is purely informational — it
// MUST NEVER feed ranking, scoring, or which model wins a role; only the
// live discovered price (D6) is authoritative for that.
type ReferencePricer interface {
	// Lookup returns the reference input/output price per 1M tokens for
	// providerID+slug, and ok=false when no usable match exists — including
	// when the reference catalog itself is unavailable or malformed (see
	// fileReferencePricer), or when the matched entry's own reference cost is
	// zero (not useful as a "roughly what it costs" figure).
	Lookup(providerID, slug string) (inputPerM, outputPerM float64, ok bool)
}

// referenceModelCost/referenceModelEntry/referenceProviderEntry/
// referenceCatalog mirror the models.dev-style shape OpenCode itself
// maintains at ~/.cache/opencode/models.json: one JSON object keyed by
// providerID, each with a "models" object keyed by model id, each carrying
// {"cost": {"input": <float>, "output": <float>, ...}} in USD per 1M tokens.
// Cost fields decode as `any` rather than float64: encoding/json never
// fails a document decode over a single field's JSON type (a JSON object
// value always fits `any`), so a malformed per-entry cost (wrong type, or
// simply absent) can be validated and skipped individually at Lookup time —
// via a plain type assertion — without ever risking the whole-document
// decode error path being conflated with one bad entry (the same
// per-entry-drop spirit as discover.go's discoverRecord, design D4).
// Unknown extra fields decode and are silently ignored (same PATH-hijack
// posture as discoverRecord).
type referenceModelCost struct {
	Input  any `json:"input"`
	Output any `json:"output"`
}

type referenceModelEntry struct {
	Cost referenceModelCost `json:"cost"`
}

type referenceProviderEntry struct {
	Models map[string]referenceModelEntry `json:"models"`
}

type referenceCatalog map[string]referenceProviderEntry

// referenceMaxFileSize bounds the file read (io.LimitReader) — the real
// file is ~4MB, this is generous headroom against a corrupted/huge file
// (mirrors discover.go's discoverMaxStdout guard/threat-matrix posture).
const referenceMaxFileSize = 32 << 20 // 32 MiB

// referenceLoadCall represents one in-flight catalog load shared by every
// concurrent Lookup caller within the same window, mirroring discoverCall's
// single-flight shape in discover.go.
type referenceLoadCall struct {
	done    chan struct{}
	catalog referenceCatalog
	err     error
}

// fileReferencePricer is the default ReferencePricer: it reads OpenCode's
// own static reference-pricing cache at ~/.cache/opencode/models.json,
// parses it once, and caches the parsed result in-process with the SAME
// TTL convention discover.go's execDiscoverer already uses for its own
// cache (discoverSuccessTTL/discoverFailureTTL) — reused directly rather
// than reinvented — plus the same single-flight shape.
//
// Every failure — home dir unresolvable, file absent, unreadable, oversize,
// or malformed JSON — is non-fatal: logged via slog.Warn ONCE per cache
// refresh (not per Lookup call), and every Lookup call returns ok=false
// until the next refresh (design D4-style failure taxonomy). A per-entry
// malformed record (missing cost, wrong types) is skipped individually,
// never fatal to the whole parse.
type fileReferencePricer struct {
	// pathFn resolves the catalog path; overridable in tests so they never
	// depend on the real home directory (t.TempDir() seam).
	pathFn func() (string, error)
	// maxFileSize bounds the file read; defaults to referenceMaxFileSize,
	// overridable in tests so the oversize-file case doesn't need a real
	// 32 MiB fixture.
	maxFileSize int64

	mu        sync.Mutex
	cachedAt  time.Time
	cached    referenceCatalog
	cachedErr error
	inFlight  *referenceLoadCall
	warned    bool
}

// newFileReferencePricer builds the default ReferencePricer, wired to the
// real ~/.cache/opencode/models.json path.
func newFileReferencePricer() *fileReferencePricer {
	return &fileReferencePricer{
		pathFn:      defaultReferencePricePath,
		maxFileSize: referenceMaxFileSize,
	}
}

// defaultReferencePricePath resolves ~/.cache/opencode/models.json — the
// same path on every OS (filepath.Join(home, ".cache", "opencode",
// "models.json")).
func defaultReferencePricePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	return filepath.Join(home, ".cache", "opencode", "models.json"), nil
}

// Lookup implements ReferencePricer.
func (p *fileReferencePricer) Lookup(providerID, slug string) (inputPerM, outputPerM float64, ok bool) {
	catalog, err := p.catalog()
	if err != nil || catalog == nil {
		return 0, 0, false
	}

	entry, found := lookupProviderCaseInsensitive(catalog, providerID)
	if !found {
		return 0, 0, false
	}
	model, found := entry.Models[slug]
	if !found {
		return 0, 0, false
	}

	// Per-entry validation (design D4-style per-entry drop): a missing cost
	// object leaves both fields nil (`any` zero value); a wrong-typed value
	// (e.g. a string) fails the type assertion. Either way this single
	// malformed record is skipped without affecting any sibling entry.
	input, inputOK := model.Cost.Input.(float64)
	output, outputOK := model.Cost.Output.(float64)
	if !inputOK || !outputOK {
		return 0, 0, false
	}
	if input == 0 && output == 0 {
		// Not useful as a "roughly what it costs" reference — same
		// zero-cost-is-not-a-price stance as discover.go's flat-rate gate.
		return 0, 0, false
	}
	return input, output, true
}

// lookupProviderCaseInsensitive matches providerID case-insensitively
// (design D9's matchesProviderScope convention) against the catalog's
// top-level provider keys.
func lookupProviderCaseInsensitive(catalog referenceCatalog, providerID string) (referenceProviderEntry, bool) {
	if entry, ok := catalog[providerID]; ok {
		return entry, true
	}
	for id, entry := range catalog {
		if strings.EqualFold(id, providerID) {
			return entry, true
		}
	}
	return referenceProviderEntry{}, false
}

// catalog returns the cached parsed catalog, refreshing it from disk when
// the TTL has expired, single-flighted so concurrent Lookup calls share one
// load instead of racing N file reads (mirrors execDiscoverer.Discover's
// cache/single-flight shape in discover.go exactly).
func (p *fileReferencePricer) catalog() (referenceCatalog, error) {
	p.mu.Lock()
	if !p.cachedAt.IsZero() {
		ttl := discoverSuccessTTL
		if p.cachedErr != nil {
			ttl = discoverFailureTTL
		}
		if time.Since(p.cachedAt) < ttl {
			catalog, err := p.cached, p.cachedErr
			p.mu.Unlock()
			return catalog, err
		}
	}
	if call := p.inFlight; call != nil {
		p.mu.Unlock()
		<-call.done
		return call.catalog, call.err
	}

	call := &referenceLoadCall{done: make(chan struct{})}
	p.inFlight = call
	p.mu.Unlock()

	catalog, err := p.load()

	p.mu.Lock()
	p.cached, p.cachedErr, p.cachedAt = catalog, err, time.Now()
	p.inFlight = nil
	if err != nil {
		if !p.warned {
			slog.Warn("reference price catalog unavailable, continuing without reference pricing", "error", err)
			p.warned = true
		}
	} else {
		p.warned = false
	}
	p.mu.Unlock()

	call.catalog, call.err = catalog, err
	close(call.done)
	return catalog, err
}

// load reads and parses the catalog file. Cost fields decode as `any`
// (see referenceModelCost), so a single per-entry malformed value can never
// trigger a document-level decode error here — every error load returns is
// a genuine whole-document failure: unreadable file, oversize file, or a
// JSON syntax error (design D4-style per-entry drop vs. whole-parse
// failure; per-entry validation happens in Lookup).
func (p *fileReferencePricer) load() (referenceCatalog, error) {
	path, err := p.pathFn()
	if err != nil {
		return nil, err
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening reference price catalog: %w", err)
	}
	defer f.Close()

	limit := p.maxFileSize
	if limit <= 0 {
		limit = referenceMaxFileSize
	}
	raw, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, fmt.Errorf("reading reference price catalog: %w", err)
	}
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("reference price catalog exceeded %d byte limit, treating as malformed", limit)
	}

	var catalog referenceCatalog
	if err := json.Unmarshal(raw, &catalog); err != nil {
		return nil, fmt.Errorf("parsing reference price catalog: %w", err)
	}
	return catalog, nil
}

// defaultReferencePricer is the package-level fileReferencePricer singleton
// used by Client.referencePricer() when Client.ReferencePricer is nil.
// Sharing one instance (rather than constructing one per Client/call) lets
// the in-process TTL cache and single-flight apply across every ListModels
// call made by this process, not just within one Client — mirrors
// defaultDiscoverer's rationale in discover.go exactly.
var defaultReferencePricer ReferencePricer = newFileReferencePricer()

// noopReferencePricer never finds a match. It is the resolved default under
// `go test` (see Client.referencePricer): mirrors noopDiscoverer's rationale
// in discover.go — existing Client{} literals across many test files never
// set ReferencePricer explicitly and must stay hermetic, never touching the
// real filesystem/home directory.
type noopReferencePricer struct{}

func (noopReferencePricer) Lookup(providerID, slug string) (float64, float64, bool) {
	return 0, 0, false
}

// referencePricer resolves the ReferencePricer for a merge call: an
// explicitly-set Client.ReferencePricer always wins; otherwise it is the
// real default fileReferencePricer in production, or the safe
// noopReferencePricer under `go test` — mirrors Client.discoverer() exactly
// (discover.go).
func (c *Client) referencePricer() ReferencePricer {
	if c.ReferencePricer != nil {
		return c.ReferencePricer
	}
	if testing.Testing() {
		return noopReferencePricer{}
	}
	return defaultReferencePricer
}

// applyReferencePricing attaches reference (list) pricing to merged models
// whose live price is exactly $0 because they came from local `opencode`
// CLI discovery (PriceSource=="opencode-cli") — a $0 price sourced any other
// way (e.g. an OpenRouter-only entry never touched by discovery) is left
// untouched (design: reference pricing for flat-rate/OAuth providers). This
// is purely an additive display annotation: it never modifies Pricing or
// PriceSource, and a missing reference match is silent (no error, no log
// spam per-model) — every ranking/scoring decision still runs entirely off
// Pricing, unaffected by this call.
func applyReferencePricing(pricer ReferencePricer, models []EnrichedModel) []EnrichedModel {
	for i := range models {
		m := &models[i]
		if m.PriceSource != "opencode-cli" {
			continue
		}
		if m.Pricing == nil || m.Pricing.Prompt != 0 || m.Pricing.Completion != 0 {
			continue
		}

		inputPerM, outputPerM, ok := pricer.Lookup(m.ProviderID, slugOf(*m))
		if !ok {
			continue
		}
		in, out := inputPerM, outputPerM
		m.ReferenceInputPerM = &in
		m.ReferenceOutputPerM = &out
	}
	return models
}
