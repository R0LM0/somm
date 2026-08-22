package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	ocGoURL  = "https://opencode.ai/zen/go/v1/models"
	ocZenURL = "https://opencode.ai/zen/v1/models"
	orURL    = "https://openrouter.ai/api/v1/models"
)

// ErrOCKeyMissing is returned when an OpenCode API key is required but absent.
var ErrOCKeyMissing = errors.New("OPENCODE_API_KEY not set in environment")

// NewClient creates a Client. A nil HTTPClient is interpreted as
// a default client with a 30-second timeout at request time.
func NewClient(hc *http.Client, ocKey, orKey string) *Client {
	return &Client{
		HTTPClient: hc,
		OCAPIKey:   ocKey,
		ORAPIKey:   orKey,
	}
}

var defaultHTTPClient = &http.Client{Timeout: 30 * time.Second}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return defaultHTTPClient
}

// fetchOC performs a GET request to an OpenCode models endpoint and returns
// the decoded model list. It expects a JSON object with a "data" field or a
// JSON array directly. Retries once on 5xx or network errors.
func (c *Client) fetchOC(ctx context.Context, url, label string) ([]OCModel, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.OCAPIKey)

		resp, err := c.httpClient().Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 500 {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<10))
			lastErr = fmt.Errorf("OpenCode %s API returned %d: %s — %s", label, resp.StatusCode, http.StatusText(resp.StatusCode), strings.TrimSpace(string(body)))
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<10))
			return nil, fmt.Errorf("OpenCode %s API returned %d: %s — %s", label, resp.StatusCode, http.StatusText(resp.StatusCode), strings.TrimSpace(string(body)))
		}

		return decodeBody[OCModel](resp.Body, "OpenCode")
	}
	return nil, lastErr
}

// fetchOpenRouter performs a GET request to an OpenRouter models endpoint and
// returns the decoded model list. It expects a JSON object with a "data" field
// or a JSON array directly. Authorization is only sent when an API key is set.
// Retries once on 5xx or network errors.
func (c *Client) fetchOpenRouter(ctx context.Context, url string) ([]ORModel, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		for k, v := range BuildOpenRouterHeaders(c.ORAPIKey) {
			req.Header[k] = v
		}

		resp, err := c.httpClient().Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 500 {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<10))
			lastErr = fmt.Errorf("OpenRouter API returned %d: %s — %s", resp.StatusCode, http.StatusText(resp.StatusCode), strings.TrimSpace(string(body)))
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<10))
			return nil, fmt.Errorf("OpenRouter API returned %d: %s — %s", resp.StatusCode, http.StatusText(resp.StatusCode), strings.TrimSpace(string(body)))
		}

		return decodeBody[ORModel](resp.Body, "OpenRouter")
	}
	return nil, lastErr
}

func decodeBody[T any](r io.Reader, source string) ([]T, error) {
	b, err := io.ReadAll(io.LimitReader(r, 1<<20)) // 1 MiB limit
	if err != nil {
		return nil, fmt.Errorf("reading %s response: %w", source, err)
	}

	var arr []T
	if err := json.Unmarshal(b, &arr); err == nil {
		return arr, nil
	}

	var obj struct {
		Data []T `json:"data"`
	}
	if err := json.Unmarshal(b, &obj); err != nil {
		return nil, fmt.Errorf("decoding %s response: %w", source, err)
	}
	return obj.Data, nil
}

// ListModels fetches models from the requested OpenCode subscription(s). When
// subscription is "both" the Go and Zen requests run in parallel. Identical
// ocId values are merged with subscription set to "both". If enrich is true and
// an OpenRouter API key is available, models are cross-referenced with
// OpenRouter benchmarks. OpenRouter failures are logged and ignored so the
// original models are still returned.
func (c *Client) ListModels(ctx context.Context, subscription string, enrich bool) ([]EnrichedModel, error) {
	if c.OCAPIKey == "" {
		return nil, ErrOCKeyMissing
	}

	var goModels, zenModels []OCModel
	var goErr, zenErr error
	var discovered []DiscoveredModel
	var discErr error
	var wg sync.WaitGroup

	if subscription == "go" || subscription == "both" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			goModels, goErr = c.fetchOC(ctx, ocGoURL, "Go")
		}()
	}
	if subscription == "zen" || subscription == "both" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			zenModels, zenErr = c.fetchOC(ctx, ocZenURL, "Zen")
		}()
	}
	// Local `opencode` CLI discovery (design D1-D4) runs alongside the OC
	// Go/Zen fetches in the same WaitGroup — max, not sum, of failure impact.
	// Every discovery error is non-fatal: warn and continue with exactly
	// today's OC Go/Zen + OpenRouter result (design D4, Requirement: Graceful
	// Degradation).
	wg.Add(1)
	go func() {
		defer wg.Done()
		discovered, discErr = c.discoverer().Discover(ctx)
	}()
	wg.Wait()

	if goErr != nil {
		slog.Error("OC Go fetch failed", "subscription", "go", "error", goErr)
	}
	if zenErr != nil {
		slog.Error("OC Zen fetch failed", "subscription", "zen", "error", zenErr)
	}
	if discErr != nil {
		slog.Warn("provider discovery failed, continuing with existing catalog", "error", discErr)
	}

	seen := make(map[string]*EnrichedModel)

	addModels := func(models []OCModel, sub string) {
		for _, m := range models {
			existing, ok := seen[m.ID]
			if ok {
				existing.Subscription = "both"
				continue
			}
			seen[m.ID] = &EnrichedModel{
				OCID:         m.ID,
				OCName:       deriveDisplayName(m.ID),
				OCProvider:   deriveProvider(m.ID),
				Subscription: sub,
			}
		}
	}

	if subscription == "go" || subscription == "both" {
		addModels(goModels, "go")
	}
	if subscription == "zen" || subscription == "both" {
		addModels(zenModels, "zen")
	}

	if len(seen) == 0 {
		return nil, errors.New("No models fetched — both subscriptions may have failed")
	}

	models := make([]EnrichedModel, 0, len(seen))
	for _, m := range seen {
		models = append(models, *m)
	}

	models = mergeDiscovered(models, discovered)
	models = applyReferencePricing(c.referencePricer(), models)

	if enrich {
		orModels, err := c.fetchOpenRouter(ctx, orURL)
		if err == nil {
			enrichWithOpenRouter(models, orModels)
		} else {
			slog.Warn("OpenRouter enrichment failed, returning models without benchmarks", "error", err)
		}
	}

	sort.Slice(models, func(i, j int) bool {
		return models[i].OCID < models[j].OCID
	})
	return models, nil
}

// ListORModels fetches the full OpenRouter model list and optionally filters it
// by a substring matched against model IDs and names (case-insensitive).
func (c *Client) ListORModels(ctx context.Context, filter string) ([]ORModel, error) {
	models, err := c.fetchOpenRouter(ctx, orURL)
	if err != nil {
		return nil, err
	}

	if filter == "" {
		return models, nil
	}

	q := strings.ToLower(filter)
	filtered := make([]ORModel, 0)
	for _, m := range models {
		if strings.Contains(strings.ToLower(m.ID), q) || strings.Contains(strings.ToLower(m.Name), q) {
			filtered = append(filtered, m)
		}
	}
	return filtered, nil
}

// mergeKey builds the dedupe identity combining provider identity and bare
// model slug (design D5): two providers publishing the same slug (e.g.
// OpenAI's "gpt-5.6" reached via the CLI vs OC Go's own "gpt-5.6") are
// genuinely different purchases at different prices and must never collide.
func mergeKey(providerID, slug string) string {
	return providerID + "/" + slug
}

// normalizeDiscoveredProviderID canonicalizes the CLI's opencode-family
// providerIDs to "opencode" so a CLI-discovered OpenCode-hosted model merges
// with (and CLI-price-overwrites) its OC Go/Zen-sourced counterpart. Every
// other providerID (openai, kimi-for-coding, ...) passes through unchanged
// (design D5).
func normalizeDiscoveredProviderID(providerID string) string {
	switch strings.ToLower(providerID) {
	case "opencode", "opencode-go", "opencode-zen":
		return "opencode"
	default:
		return providerID
	}
}

// deriveDiscoveredProviderName renders a human-readable name for a
// (normalized) CLI providerID. "opencode" gets the same display name used
// elsewhere in this package; anything else is title-cased word-by-word on
// its dash separators (design does not specify a lookup table for arbitrary
// CLI providerIDs, unlike the OC-model-ID prefix table in provider.go).
func deriveDiscoveredProviderName(providerID string) string {
	if providerID == "opencode" {
		return "OpenCode"
	}
	words := strings.Split(providerID, "-")
	for i, w := range words {
		words[i] = titleCaseFirst(w)
	}
	return strings.Join(words, " ")
}

// mergeDiscovered merges CLI-discovered models into models built from OC
// Go/Zen (design Data Flow: Discoverer -> mergeDiscovered ->
// enrichWithOpenRouter). HTTP-sourced (OC Go/Zen) entries implicitly carry
// ProviderID "opencode" for dedupe purposes (design D5). A discovered model
// whose mergeKey collides with an existing entry overwrites that entry's
// pricing and sets PriceSource (design D6) instead of appending a duplicate;
// otherwise it becomes a new entry, namespaced by provider unless its
// (normalized) ProviderID is "opencode" (design D5's OCID rule). Called with
// an empty/nil discovered slice, models is returned unchanged (Requirement:
// Behavior Unchanged When Discovery Is Absent).
func mergeDiscovered(models []EnrichedModel, discovered []DiscoveredModel) []EnrichedModel {
	byKey := make(map[string]int, len(models))
	for i := range models {
		byKey[mergeKey("opencode", models[i].OCID)] = i
	}

	for _, dm := range discovered {
		providerID := normalizeDiscoveredProviderID(dm.ProviderID)
		providerName := deriveDiscoveredProviderName(providerID)
		key := mergeKey(providerID, dm.ID)

		pricing := &Money{
			Prompt:     dm.InputPerM / 1e6,
			Completion: dm.OutputPerM / 1e6,
		}
		if dm.CacheReadPerM != nil {
			cacheRead := *dm.CacheReadPerM / 1e6
			pricing.InputCacheRead = &cacheRead
		}

		if idx, ok := byKey[key]; ok {
			m := &models[idx]
			m.ProviderID = providerID
			m.ProviderName = providerName
			m.ModelSlug = dm.ID
			m.Pricing = pricing
			m.PriceSource = "opencode-cli"
			if dm.ContextLength != nil {
				m.ContextLength = dm.ContextLength
			}
			continue
		}

		ocid := dm.ID
		if providerID != "opencode" {
			ocid = providerID + "/" + dm.ID
		}
		name := dm.Name
		if name == "" {
			name = deriveDisplayName(dm.ID)
		}
		models = append(models, EnrichedModel{
			OCID:          ocid,
			OCName:        name,
			OCProvider:    providerName,
			ProviderID:    providerID,
			ProviderName:  providerName,
			ModelSlug:     dm.ID,
			Pricing:       pricing,
			ContextLength: dm.ContextLength,
			PriceSource:   "opencode-cli",
		})
		byKey[key] = len(models) - 1
	}
	return models
}
