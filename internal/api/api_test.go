package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func rewriteTransport(server *httptest.Server) *http.Client {
	u, _ := url.Parse(server.URL)
	return &http.Client{
		// A transport of its own, not the shared http.DefaultTransport: that
		// global pools keep-alive connections by host:port, and two
		// httptest.Server instances across different tests can end up
		// reusing the same OS-assigned ephemeral port, letting one test's
		// pooled connection leak into another test's requests.
		Transport: &urlRewriteTransport{host: u.Host, inner: &http.Transport{}},
	}
}

type urlRewriteTransport struct {
	host  string
	inner http.RoundTripper
}

func (t *urlRewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme = "http"
	req.URL.Host = t.host
	return t.inner.RoundTrip(req)
}

func newClientWithServer(server *httptest.Server, ocKey, orKey, kimiKey string) *Client {
	return &Client{
		HTTPClient: rewriteTransport(server),
		OCAPIKey:   ocKey,
		ORAPIKey:   orKey,
		KIMAPIKey:  kimiKey,
	}
}

func TestNewClient(t *testing.T) {
	custom := &http.Client{Timeout: 0}
	c := NewClient(custom, "oc-key", "or-key", "kimi-key")
	if c == nil {
		t.Fatal("NewClient returned nil")
	}
	if c.HTTPClient != custom {
		t.Error("NewClient did not store custom HTTP client")
	}
	if c.OCAPIKey != "oc-key" {
		t.Errorf("OCAPIKey = %q, want oc-key", c.OCAPIKey)
	}
	if c.ORAPIKey != "or-key" {
		t.Errorf("ORAPIKey = %q, want or-key", c.ORAPIKey)
	}
	if c.KIMAPIKey != "kimi-key" {
		t.Errorf("KIMAPIKey = %q, want kimi-key", c.KIMAPIKey)
	}

	cNil := NewClient(nil, "oc-key", "or-key", "")
	if cNil.HTTPClient != nil {
		t.Error("NewClient(nil, ...) should keep nil HTTPClient (default resolved at request time)")
	}
	if cNil.KIMAPIKey != "" {
		t.Errorf("KIMAPIKey = %q, want empty", cNil.KIMAPIKey)
	}
}

func TestFetchOC(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantErrMsg string
		wantModels []OCModel
	}{
		{
			name:   "success object wrapper",
			status: http.StatusOK,
			body:   `{"data":[{"id":"go-1","object":"model","created":1,"owned_by":"test"}]}`,
			wantModels: []OCModel{
				{ID: "go-1", Object: "model", Created: 1, OwnedBy: "test"},
			},
		},
		{
			name:   "success array body",
			status: http.StatusOK,
			body:   `[{"id":"zen-1","object":"model","created":2,"owned_by":"test"}]`,
			wantModels: []OCModel{
				{ID: "zen-1", Object: "model", Created: 2, OwnedBy: "test"},
			},
		},
		{
			name:       "error status",
			status:     http.StatusInternalServerError,
			body:       `internal error`,
			wantErrMsg: "OpenCode Test API returned 500: Internal Server Error",
		},
		{
			name:       "invalid JSON",
			status:     http.StatusOK,
			body:       `{`,
			wantErrMsg: "decoding OpenCode response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if auth := r.Header.Get("Authorization"); auth != "Bearer oc-key" {
					t.Errorf("Authorization = %q, want Bearer oc-key", auth)
				}
				w.WriteHeader(tt.status)
				w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			client := &Client{HTTPClient: http.DefaultClient, OCAPIKey: "oc-key"}
			models, err := client.fetchOC(context.Background(), srv.URL, "Test")
			if tt.wantErrMsg != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Fatalf("fetchOC error = %v, want containing %q", err, tt.wantErrMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(models, tt.wantModels) {
				t.Errorf("models = %#v, want %#v", models, tt.wantModels)
			}
		})
	}
}

func TestFetchOpenRouter(t *testing.T) {
	tests := []struct {
		name       string
		orKey      string
		status     int
		body       string
		wantAuth   string
		wantErrMsg string
		wantModels []ORModel
	}{
		{
			name:     "success with auth",
			orKey:    "or-key",
			status:   http.StatusOK,
			body:     `{"data":[{"id":"openai/gpt-4","name":"GPT-4"}]}`,
			wantAuth: "Bearer or-key",
			wantModels: []ORModel{
				{ID: "openai/gpt-4", Name: "GPT-4"},
			},
		},
		{
			name:     "success without auth",
			orKey:    "",
			status:   http.StatusOK,
			body:     `{"data":[{"id":"openai/gpt-3.5","name":"GPT-3.5"}]}`,
			wantAuth: "",
			wantModels: []ORModel{
				{ID: "openai/gpt-3.5", Name: "GPT-3.5"},
			},
		},
		{
			name:       "error status",
			orKey:      "or-key",
			status:     http.StatusBadGateway,
			body:       `bad gateway`,
			wantAuth:   "Bearer or-key",
			wantErrMsg: "OpenRouter API returned 502",
		},
		{
			name:       "invalid JSON",
			orKey:      "or-key",
			status:     http.StatusOK,
			body:       `not json`,
			wantAuth:   "Bearer or-key",
			wantErrMsg: "decoding OpenRouter response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				auth := r.Header.Get("Authorization")
				if auth != tt.wantAuth {
					t.Errorf("Authorization = %q, want %q", auth, tt.wantAuth)
				}
				w.WriteHeader(tt.status)
				w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			client := &Client{HTTPClient: http.DefaultClient, ORAPIKey: tt.orKey}
			models, err := client.fetchOpenRouter(context.Background(), srv.URL)
			if tt.wantErrMsg != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Fatalf("fetchOpenRouter error = %v, want containing %q", err, tt.wantErrMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(models, tt.wantModels) {
				t.Errorf("models = %#v, want %#v", models, tt.wantModels)
			}
		})
	}
}

func TestListModels(t *testing.T) {
	goBody := `{"data":[{"id":"go-1","object":"model","created":1,"owned_by":"test"},{"id":"shared-1","object":"model","created":3,"owned_by":"test"}]}`
	zenBody := `{"data":[{"id":"zen-1","object":"model","created":2,"owned_by":"test"},{"id":"shared-1","object":"model","created":3,"owned_by":"test"}]}`
	orBody := `{"data":[{"id":"go-1","name":"Go One","pricing":{"prompt":"0.000001","completion":"0.000002"},"context_length":128,"benchmarks":{"artificial_analysis":{"intelligence_index":0.8,"coding_index":0.7,"agentic_index":0.6}},"reasoning":{"supported_efforts":["low","medium","high"],"default_effort":"medium","default_enabled":true,"mandatory":false}}]}`

	baseHandler := func(goStatus, zenStatus, orStatus int, orBody string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/zen/go/v1/models":
				w.WriteHeader(goStatus)
				w.Write([]byte(goBody))
			case "/zen/v1/models":
				w.WriteHeader(zenStatus)
				w.Write([]byte(zenBody))
			case "/api/v1/models":
				w.WriteHeader(orStatus)
				w.Write([]byte(orBody))
			default:
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
		}
	}

	wantBoth := []EnrichedModel{
		{
			OCID:         "go-1",
			OCName:       "Go 1",
			OCProvider:   "Go",
			Subscription: "go",
		},
		{
			OCID:         "shared-1",
			OCName:       "Shared 1",
			OCProvider:   "Shared",
			Subscription: "both",
		},
		{
			OCID:         "zen-1",
			OCName:       "Zen 1",
			OCProvider:   "Zen",
			Subscription: "zen",
		},
	}

	tests := []struct {
		name       string
		ocKey      string
		sub        string
		enrich     bool
		goStatus   int
		zenStatus  int
		orStatus   int
		orBody     string
		wantErrMsg string
		want       []EnrichedModel
	}{
		{
			name:      "both subscriptions merged and deduplicated",
			ocKey:     "oc-key",
			sub:       "both",
			enrich:    false,
			goStatus:  http.StatusOK,
			zenStatus: http.StatusOK,
			orStatus:  http.StatusOK,
			orBody:    orBody,
			want:      wantBoth,
		},
		{
			name:      "go only",
			ocKey:     "oc-key",
			sub:       "go",
			enrich:    false,
			goStatus:  http.StatusOK,
			zenStatus: http.StatusOK,
			orStatus:  http.StatusOK,
			orBody:    orBody,
			want: []EnrichedModel{
				{OCID: "go-1", OCName: "Go 1", OCProvider: "Go", Subscription: "go"},
				{OCID: "shared-1", OCName: "Shared 1", OCProvider: "Shared", Subscription: "go"},
			},
		},
		{
			name:      "zen only",
			ocKey:     "oc-key",
			sub:       "zen",
			enrich:    false,
			goStatus:  http.StatusOK,
			zenStatus: http.StatusOK,
			orStatus:  http.StatusOK,
			orBody:    orBody,
			want: []EnrichedModel{
				{OCID: "shared-1", OCName: "Shared 1", OCProvider: "Shared", Subscription: "zen"},
				{OCID: "zen-1", OCName: "Zen 1", OCProvider: "Zen", Subscription: "zen"},
			},
		},
		{
			name:       "missing OC key",
			ocKey:      "",
			sub:        "both",
			wantErrMsg: "OPENCODE_API_KEY not set in environment",
		},
		{
			name:      "OC Go error returns Zen models",
			ocKey:     "oc-key",
			sub:       "both",
			goStatus:  http.StatusInternalServerError,
			zenStatus: http.StatusOK,
			orStatus:  http.StatusOK,
			orBody:    orBody,
			want: []EnrichedModel{
				{OCID: "shared-1", OCName: "Shared 1", OCProvider: "Shared", Subscription: "zen"},
				{OCID: "zen-1", OCName: "Zen 1", OCProvider: "Zen", Subscription: "zen"},
			},
		},
		{
			name:      "OC Zen error returns Go models",
			ocKey:     "oc-key",
			sub:       "both",
			goStatus:  http.StatusOK,
			zenStatus: http.StatusInternalServerError,
			orStatus:  http.StatusOK,
			orBody:    orBody,
			want: []EnrichedModel{
				{OCID: "go-1", OCName: "Go 1", OCProvider: "Go", Subscription: "go"},
				{OCID: "shared-1", OCName: "Shared 1", OCProvider: "Shared", Subscription: "go"},
			},
		},
		{
			name:       "both OC subscriptions fail",
			ocKey:      "oc-key",
			sub:        "both",
			goStatus:   http.StatusInternalServerError,
			zenStatus:  http.StatusInternalServerError,
			orStatus:   http.StatusOK,
			orBody:     orBody,
			wantErrMsg: "No models fetched — both subscriptions may have failed",
		},
		{
			name:      "enrich true applies OpenRouter data",
			ocKey:     "oc-key",
			sub:       "both",
			enrich:    true,
			goStatus:  http.StatusOK,
			zenStatus: http.StatusOK,
			orStatus:  http.StatusOK,
			orBody:    orBody,
			want: func() []EnrichedModel {
				m := make([]EnrichedModel, len(wantBoth))
				copy(m, wantBoth)
				m[0].ORID = strPtr("go-1")
				m[0].ORName = strPtr("Go One")
				m[0].Pricing = &Money{Prompt: 0.000001, Completion: 0.000002}
				ctxLen := int64(128)
				m[0].ContextLength = &ctxLen
				m[0].Benchmarks = ModelBenchmarks{
					Intelligence: float64Ptr(0.8),
					Coding:       float64Ptr(0.7),
					Agentic:      float64Ptr(0.6),
				}
				m[0].Reasoning = &ModelReasoning{
					SupportedEfforts: []string{"low", "medium", "high"},
					DefaultEffort:    strPtr("medium"),
					Mandatory:        false,
					DefaultEnabled:   true,
				}
				return m
			}(),
		},
		{
			name:      "enrich false skips OpenRouter",
			ocKey:     "oc-key",
			sub:       "both",
			enrich:    false,
			goStatus:  http.StatusOK,
			zenStatus: http.StatusOK,
			orStatus:  http.StatusInternalServerError,
			orBody:    `{"data":[]}`,
			want:      wantBoth,
		},
		{
			name:      "OpenRouter failure degrades gracefully",
			ocKey:     "oc-key",
			sub:       "both",
			enrich:    true,
			goStatus:  http.StatusOK,
			zenStatus: http.StatusOK,
			orStatus:  http.StatusInternalServerError,
			orBody:    `error`,
			want:      wantBoth,
		},
		{
			name:      "OpenRouter JSON decode failure degrades gracefully",
			ocKey:     "oc-key",
			sub:       "both",
			enrich:    true,
			goStatus:  http.StatusOK,
			zenStatus: http.StatusOK,
			orStatus:  http.StatusOK,
			orBody:    `{bad`,
			want:      wantBoth,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(baseHandler(tt.goStatus, tt.zenStatus, tt.orStatus, tt.orBody))
			defer srv.Close()

			client := newClientWithServer(srv, tt.ocKey, "or-key", "")
			models, err := client.ListModels(context.Background(), tt.sub, tt.enrich)
			if tt.wantErrMsg != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Fatalf("ListModels error = %v, want containing %q", err, tt.wantErrMsg)
				}
				if tt.wantErrMsg == "OPENCODE_API_KEY not set in environment" && !errors.Is(err, ErrOCKeyMissing) {
					t.Errorf("ListModels error is not ErrOCKeyMissing: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(models) != len(tt.want) {
				t.Fatalf("len(models) = %d, want %d", len(models), len(tt.want))
			}
			sortEnrichedByID(models)
			wantSorted := make([]EnrichedModel, len(tt.want))
			copy(wantSorted, tt.want)
			sortEnrichedByID(wantSorted)
			if !reflect.DeepEqual(models, wantSorted) {
				t.Errorf("models = %#v, want %#v", models, wantSorted)
			}
		})
	}
}

func TestListORModels(t *testing.T) {
	orBody := `{"data":[{"id":"openai/gpt-4","name":"GPT-4"},{"id":"openai/gpt-3.5","name":"GPT-3.5"},{"id":"anthropic/claude-3","name":"Claude 3"}]}`

	tests := []struct {
		name       string
		filter     string
		orStatus   int
		orBody     string
		wantErrMsg string
		want       []ORModel
	}{
		{
			name:     "no filter returns all",
			filter:   "",
			orStatus: http.StatusOK,
			orBody:   orBody,
			want: []ORModel{
				{ID: "openai/gpt-4", Name: "GPT-4"},
				{ID: "openai/gpt-3.5", Name: "GPT-3.5"},
				{ID: "anthropic/claude-3", Name: "Claude 3"},
			},
		},
		{
			name:     "filter by id substring",
			filter:   "claude",
			orStatus: http.StatusOK,
			orBody:   orBody,
			want: []ORModel{
				{ID: "anthropic/claude-3", Name: "Claude 3"},
			},
		},
		{
			name:     "filter by name substring",
			filter:   "GPT",
			orStatus: http.StatusOK,
			orBody:   orBody,
			want: []ORModel{
				{ID: "openai/gpt-4", Name: "GPT-4"},
				{ID: "openai/gpt-3.5", Name: "GPT-3.5"},
			},
		},
		{
			name:     "filter case insensitive",
			filter:   "GPT-4",
			orStatus: http.StatusOK,
			orBody:   orBody,
			want: []ORModel{
				{ID: "openai/gpt-4", Name: "GPT-4"},
			},
		},
		{
			name:     "no matches returns empty",
			filter:   "missing",
			orStatus: http.StatusOK,
			orBody:   orBody,
			want:     []ORModel{},
		},
		{
			name:       "server error returned",
			filter:     "",
			orStatus:   http.StatusBadGateway,
			orBody:     `error`,
			wantErrMsg: "OpenRouter API returned 502",
		},
		{
			name:       "invalid JSON returned",
			filter:     "",
			orStatus:   http.StatusOK,
			orBody:     `{`,
			wantErrMsg: "decoding OpenRouter response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/models" {
					t.Fatalf("unexpected path: %s", r.URL.Path)
				}
				w.WriteHeader(tt.orStatus)
				w.Write([]byte(tt.orBody))
			}))
			defer srv.Close()

			client := newClientWithServer(srv, "oc-key", "or-key", "")
			models, err := client.ListORModels(context.Background(), tt.filter)
			if tt.wantErrMsg != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Fatalf("ListORModels error = %v, want containing %q", err, tt.wantErrMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(models, tt.want) {
				t.Errorf("models = %#v, want %#v", models, tt.want)
			}
		})
	}
}

func sortEnrichedByID(models []EnrichedModel) {
	for i := 0; i < len(models)-1; i++ {
		for j := i + 1; j < len(models); j++ {
			if models[i].OCID > models[j].OCID {
				models[i], models[j] = models[j], models[i]
			}
		}
	}
}

func strPtr(s string) *string {
	return &s
}

func float64Ptr(f float64) *float64 {
	return &f
}

func int64Ptr(i int64) *int64 {
	return &i
}
