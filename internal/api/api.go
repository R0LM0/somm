package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
)

const (
	ocGoURL  = "https://opencode.ai/zen/go/v1/models"
	ocZenURL = "https://opencode.ai/zen/v1/models"
	orURL    = "https://openrouter.ai/api/v1/models"
)

// ErrOCKeyMissing is returned when an OpenCode API key is required but absent.
var ErrOCKeyMissing = errors.New("OPENCODE_API_KEY not set in environment")

// NewClient creates a Client. A nil HTTPClient is interpreted as
// http.DefaultClient at request time.
func NewClient(hc *http.Client, ocKey, orKey string) *Client {
	return &Client{
		HTTPClient: hc,
		OCAPIKey:   ocKey,
		ORAPIKey:   orKey,
	}
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

// fetchOC performs a GET request to an OpenCode models endpoint and returns
// the decoded model list. It expects a JSON object with a "data" field or a
// JSON array directly.
func (c *Client) fetchOC(ctx context.Context, url, label string) ([]OCModel, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.OCAPIKey)

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("OpenCode %s API returned %d: %s", label, resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	return decodeBody[OCModel](resp.Body, "OpenCode")
}

// fetchOpenRouter performs a GET request to an OpenRouter models endpoint and
// returns the decoded model list. It expects a JSON object with a "data" field
// or a JSON array directly. Authorization is only sent when an API key is set.
func (c *Client) fetchOpenRouter(ctx context.Context, url string) ([]ORModel, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range BuildOpenRouterHeaders(c.ORAPIKey) {
		req.Header[k] = v
	}

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("OpenRouter API returned %d", resp.StatusCode)
	}

	return decodeBody[ORModel](resp.Body, "OpenRouter")
}

func decodeBody[T any](r io.Reader, source string) ([]T, error) {
	b, err := io.ReadAll(r)
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
	var wg sync.WaitGroup

	if subscription == "go" || subscription == "both" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			goModels, _ = c.fetchOC(ctx, ocGoURL, "Go")
		}()
	}
	if subscription == "zen" || subscription == "both" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			zenModels, _ = c.fetchOC(ctx, ocZenURL, "Zen")
		}()
	}
	wg.Wait()

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

	if enrich {
		orModels, err := c.fetchOpenRouter(ctx, orURL)
		if err == nil {
			enrichWithOpenRouter(models, orModels)
		}
		// OpenRouter failures are intentionally degraded: return models without enrichment.
		_ = err
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
