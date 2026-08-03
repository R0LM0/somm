package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestExportConfig_MissingOCKey(t *testing.T) {
	client := &Client{OCAPIKey: "", ORAPIKey: ""}
	_, err := ExportConfig(context.Background(), client, mustPreset(t), "", nil)
	if err == nil {
		t.Fatal("expected error for missing OC key, got nil")
	}
}

func TestExportConfig_EmptyConfig(t *testing.T) {
	goBody := `{"data":[{"id":"deepseek-v4-pro","object":"model","created":1,"owned_by":"test"}]}`
	zenBody := `{"data":[{"id":"deepseek-v4-pro","object":"model","created":1,"owned_by":"test"}]}`
	orBody := `{"data":[{"id":"deepseek/deepseek-v4-pro","name":"DeepSeek V4 Pro","pricing":{"prompt":"0.000000435","completion":"0.000001"},"context_length":1000000,"benchmarks":{"artificial_analysis":{"intelligence_index":44.3,"coding_index":38.2,"agentic_index":41.5}}}]}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/zen/go/v1/models":
			w.Write([]byte(goBody))
		case "/zen/v1/models":
			w.Write([]byte(zenBody))
		case "/api/v1/models":
			w.Write([]byte(orBody))
		}
	}))
	defer srv.Close()

	client := &Client{
		HTTPClient: rewriteTransport(srv),
		OCAPIKey:   "test-key",
		ORAPIKey:   "test-or-key",
	}

	tmpDir := t.TempDir()
	emptyPath := filepath.Join(tmpDir, "opencode.json")

	result, err := ExportConfig(context.Background(), client, mustPreset(t), emptyPath, []string{"sdd-apply"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Path != emptyPath {
		t.Errorf("path = %q, want %q", result.Path, emptyPath)
	}

	if len(result.Changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(result.Changes))
	}

	c := result.Changes[0]
	if c.Agent != "sdd-apply" {
		t.Errorf("agent = %q, want sdd-apply", c.Agent)
	}
	if c.NewModel != "deepseek-v4-pro" {
		t.Errorf("newModel = %q, want deepseek-v4-pro", c.NewModel)
	}
	if c.Applied {
		t.Error("change should not be applied when agent is missing from config")
	}
}

func TestExportConfig_ExistingConfig(t *testing.T) {
	goBody := `{"data":[{"id":"deepseek-v4-pro","object":"model","created":1,"owned_by":"test"}]}`
	zenBody := `{"data":[{"id":"deepseek-v4-pro","object":"model","created":1,"owned_by":"test"}]}`
	orBody := `{"data":[{"id":"deepseek/deepseek-v4-pro","name":"DeepSeek V4 Pro","pricing":{"prompt":"0.000000435","completion":"0.000001"},"context_length":1000000,"benchmarks":{"artificial_analysis":{"intelligence_index":44.3,"coding_index":38.2,"agentic_index":41.5}}}]}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/zen/go/v1/models":
			w.Write([]byte(goBody))
		case "/zen/v1/models":
			w.Write([]byte(zenBody))
		case "/api/v1/models":
			w.Write([]byte(orBody))
		}
	}))
	defer srv.Close()

	client := &Client{
		HTTPClient: rewriteTransport(srv),
		OCAPIKey:   "test-key",
		ORAPIKey:   "test-or-key",
	}

	existing := map[string]any{
		"agents": map[string]any{
			"sdd-apply": map[string]any{
				"model":  "old-model",
				"prompt": "keep this prompt",
				"tools":  []any{"tool1"},
			},
			"orchestrator": map[string]any{
				"model":  "old-orch",
				"prompt": "orchestrator prompt",
			},
		},
		"permissions": map[string]any{
			"allow": []any{"bash"},
		},
	}

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "opencode.json")

	data, _ := json.Marshal(existing)
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ExportConfig(context.Background(), client, mustPreset(t), configPath, []string{"sdd-apply"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(result.Changes))
	}

	c := result.Changes[0]
	if !c.Applied {
		t.Error("change should be applied")
	}
	if c.OldModel != "old-model" {
		t.Errorf("oldModel = %q, want old-model", c.OldModel)
	}
	if c.NewModel != "deepseek-v4-pro" {
		t.Errorf("newModel = %q, want deepseek-v4-pro", c.NewModel)
	}

	// Verify the JSON output preserves non-model fields.
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result.Content), &parsed); err != nil {
		t.Fatalf("failed to parse output JSON: %v", err)
	}

	agents := parsed["agents"].(map[string]any)
	sddApply := agents["sdd-apply"].(map[string]any)
	if sddApply["model"] != "deepseek-v4-pro" {
		t.Errorf("model in output = %q, want deepseek-v4-pro", sddApply["model"])
	}
	if sddApply["prompt"] != "keep this prompt" {
		t.Errorf("prompt was overwritten: %v", sddApply["prompt"])
	}
	if sddApply["tools"] == nil {
		t.Error("tools were removed")
	}

	// Verify untouched agent is preserved.
	orch := agents["orchestrator"].(map[string]any)
	if orch["model"] != "old-orch" {
		t.Errorf("untouched orchestrator model changed: %v", orch["model"])
	}
	if orch["prompt"] != "orchestrator prompt" {
		t.Errorf("untouched orchestrator prompt changed: %v", orch["prompt"])
	}

	// Verify top-level permissions preserved.
	perms := parsed["permissions"].(map[string]any)
	if perms["allow"] == nil {
		t.Error("permissions.allow was removed")
	}
}

func TestExportConfig_NonexistentFile(t *testing.T) {
	goBody := `{"data":[{"id":"deepseek-v4-pro","object":"model","created":1,"owned_by":"test"}]}`
	zenBody := `{"data":[{"id":"deepseek-v4-pro","object":"model","created":1,"owned_by":"test"}]}`
	orBody := `{"data":[{"id":"deepseek/deepseek-v4-pro","name":"DeepSeek V4 Pro","pricing":{"prompt":"0.000000435","completion":"0.000001"},"context_length":1000000,"benchmarks":{"artificial_analysis":{"intelligence_index":44.3,"coding_index":38.2,"agentic_index":41.5}}}]}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/zen/go/v1/models":
			w.Write([]byte(goBody))
		case "/zen/v1/models":
			w.Write([]byte(zenBody))
		case "/api/v1/models":
			w.Write([]byte(orBody))
		}
	}))
	defer srv.Close()

	client := &Client{
		HTTPClient: rewriteTransport(srv),
		OCAPIKey:   "test-key",
		ORAPIKey:   "test-or-key",
	}

	result, err := ExportConfig(context.Background(), client, mustPreset(t), "/nonexistent/path/opencode.json", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should produce valid JSON even with empty config.
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result.Content), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
}

func TestFormatExportResult(t *testing.T) {
	result := ExportResult{
		Content: `{"agents":{"sdd-apply":{"model":"deepseek-v4-pro"}}}`,
		Path:    `C:\Users\test\.config\opencode\opencode.json`,
		Changes: []ConfigChange{
			{Agent: "sdd-apply", OldModel: "old-model", NewModel: "deepseek-v4-pro", Applied: true},
			{Agent: "orchestrator", NewModel: "deepseek-v4-pro", Applied: false},
		},
	}

	text := FormatExportResult(result)

	if !contains(text, "Cambios a aplicar:") {
		t.Error("missing changes header")
	}
	if !contains(text, "old-model → deepseek-v4-pro") {
		t.Error("missing old→new transition")
	}
	if !contains(text, "(no configurado) → deepseek-v4-pro") {
		t.Error("missing no-configured transition")
	}
	if !contains(text, "[agente no encontrado en config]") {
		t.Error("missing not-found marker")
	}
	if !contains(text, "IMPORTANTE") {
		t.Error("missing safety warning")
	}
}

func TestDetectConfigPath(t *testing.T) {
	path := detectConfigPath()
	if path == "" {
		t.Error("detectConfigPath returned empty string")
	}
	if !contains(path, "opencode.json") {
		t.Errorf("unexpected path format: %s", path)
	}
}
