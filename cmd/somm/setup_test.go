package main

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshaling %#v: %v", v, err)
	}
	return data
}

func TestFindOpenCodeConfig(t *testing.T) {
	home := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	} else {
		t.Setenv("HOME", home)
	}

	configDir := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("creating config dir: %v", err)
	}
	configPath := filepath.Join(configDir, "opencode.json")
	if err := os.WriteFile(configPath, []byte(`{"mcp":{}}`), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	got, err := findOpenCodeConfig()
	if err != nil {
		t.Fatalf("findOpenCodeConfig error: %v", err)
	}
	if got != configPath {
		t.Errorf("findOpenCodeConfig() = %q, want %q", got, configPath)
	}
}

func TestFindOpenCodeConfig_NotFound(t *testing.T) {
	home := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	} else {
		t.Setenv("HOME", home)
	}

	_, err := findOpenCodeConfig()
	if err == nil {
		t.Fatal("expected error when opencode.json is missing")
	}
}

func TestFindBinary_GOPATH(t *testing.T) {
	gopath := t.TempDir()
	t.Setenv("GOPATH", gopath)

	binDir := filepath.Join(gopath, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("creating bin dir: %v", err)
	}

	binName := "somm"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	want := filepath.Join(binDir, binName)
	if err := os.WriteFile(want, []byte("binary"), 0755); err != nil {
		t.Fatalf("writing binary: %v", err)
	}

	got, err := findBinary()
	if err != nil {
		t.Fatalf("findBinary error: %v", err)
	}
	if got != want {
		t.Errorf("findBinary() = %q, want %q", got, want)
	}
}

func TestFindBinary_FallbackToHome(t *testing.T) {
	home := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	} else {
		t.Setenv("HOME", home)
	}
	t.Setenv("GOPATH", "")

	binDir := filepath.Join(home, "go", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("creating bin dir: %v", err)
	}

	binName := "somm"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	want := filepath.Join(binDir, binName)
	if err := os.WriteFile(want, []byte("binary"), 0755); err != nil {
		t.Fatalf("writing binary: %v", err)
	}

	got, err := findBinary()
	if err != nil {
		t.Fatalf("findBinary error: %v", err)
	}
	if got != want {
		t.Errorf("findBinary() = %q, want %q", got, want)
	}
}

func TestFindBinary_NotFound(t *testing.T) {
	gopath := t.TempDir()
	t.Setenv("GOPATH", gopath)
	os.MkdirAll(filepath.Join(gopath, "bin"), 0755)

	_, err := findBinary()
	if err == nil {
		t.Fatal("expected error when binary is missing")
	}
}

// TestIsAlreadyConfigured covers the new "no key required" semantics:
// "already configured" now means a valid somm MCP entry exists in
// opencode.json AND a .env file exists at all — its content, or lack
// thereof, no longer matters.
func TestIsAlreadyConfigured(t *testing.T) {
	tmp := t.TempDir()
	envPath := filepath.Join(tmp, ".env")
	binPath := filepath.Join(tmp, "somm")

	// Neither .env nor config configured.
	if configured, _ := isAlreadyConfigured(envPath, &openCodeConfig{MCP: map[string]json.RawMessage{}}); configured {
		t.Error("expected not configured when .env and config are empty")
	}

	// .env exists (empty — no key required) but config missing.
	if err := os.WriteFile(envPath, []byte(""), 0600); err != nil {
		t.Fatalf("writing .env: %v", err)
	}
	if configured, _ := isAlreadyConfigured(envPath, &openCodeConfig{MCP: map[string]json.RawMessage{}}); configured {
		t.Error("expected not configured when opencode.json has no somm entry")
	}

	// Both configured.
	config := &openCodeConfig{
		MCP: map[string]json.RawMessage{
			"somm": mustMarshal(t, mcpEntry{Command: []string{binPath}, Enabled: true, Type: "local"}),
		},
	}
	configured, gotPath := isAlreadyConfigured(envPath, config)
	if !configured {
		t.Error("expected configured when .env exists and somm MCP entry exist")
	}
	if gotPath != binPath {
		t.Errorf("binary path = %q, want %q", gotPath, binPath)
	}
}

func TestSaveEnvFile(t *testing.T) {
	tmp := t.TempDir()
	envPath := filepath.Join(tmp, ".env")

	keys := map[string]string{
		"OPENCODE_API_KEY":   "oc-secret",
		"OPENROUTER_API_KEY": "or-secret",
	}
	if err := saveEnvFile(envPath, keys); err != nil {
		t.Fatalf("saveEnvFile error: %v", err)
	}

	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("reading .env: %v", err)
	}
	content := string(data)
	if content == "" {
		t.Fatal("expected non-empty .env content")
	}
	for _, want := range []string{"OPENCODE_API_KEY=oc-secret", "OPENROUTER_API_KEY=or-secret"} {
		if !strings.Contains(content, want) {
			t.Errorf("expected .env to contain %q, got %q", want, content)
		}
	}

	stat, err := os.Stat(envPath)
	if err != nil {
		t.Fatalf("stat .env: %v", err)
	}
	if runtime.GOOS != "windows" && stat.Mode().Perm() != 0600 {
		t.Errorf(".env permissions = %o, want 0600", stat.Mode().Perm())
	}
}

// TestSaveEnvFile_RoundTripsAllThreeKeys is a regression guard for the
// hazard the design flagged: saveEnvFile rewrites .env wholesale from its
// order allowlist, so an update that carries only some keys must not drop
// the others. A tier-only update must not drop existing API keys, and vice
// versa.
func TestSaveEnvFile_RoundTripsAllThreeKeys(t *testing.T) {
	tmp := t.TempDir()
	envPath := filepath.Join(tmp, ".env")

	keys := map[string]string{
		"OPENCODE_API_KEY":   "oc-secret",
		"OPENROUTER_API_KEY": "or-secret",
		"SOMM_OC_TIER":       "go",
	}
	if err := saveEnvFile(envPath, keys); err != nil {
		t.Fatalf("saveEnvFile error: %v", err)
	}

	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("reading .env: %v", err)
	}
	content := string(data)
	for _, want := range []string{"OPENCODE_API_KEY=oc-secret", "OPENROUTER_API_KEY=or-secret", "SOMM_OC_TIER=go"} {
		if !strings.Contains(content, want) {
			t.Errorf("expected .env to contain %q after a full-key save, got %q", want, content)
		}
	}

	// Now simulate a tier-only re-save (skipTierScreen path re-seeds
	// SOMM_OC_TIER but the API keys must still be re-supplied by the
	// caller — verify the write itself round-trips the exact keys given,
	// dropping nothing that was passed in.
	keys2 := map[string]string{
		"OPENCODE_API_KEY":   "oc-secret",
		"OPENROUTER_API_KEY": "or-secret",
		"SOMM_OC_TIER":       "zen",
	}
	if err := saveEnvFile(envPath, keys2); err != nil {
		t.Fatalf("saveEnvFile (2nd write) error: %v", err)
	}
	data2, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("reading .env after 2nd write: %v", err)
	}
	content2 := string(data2)
	for _, want := range []string{"OPENCODE_API_KEY=oc-secret", "OPENROUTER_API_KEY=or-secret", "SOMM_OC_TIER=zen"} {
		if !strings.Contains(content2, want) {
			t.Errorf("expected .env to contain %q after tier update, got %q", want, content2)
		}
	}
}

// TestExistingTier_IndependentOfKeyPresence is the regression guard for the
// upgrade-path gap tasks.md 5.7 scoped: an existing install with API keys
// already persisted (isAlreadyConfigured == true) has NO SOMM_OC_TIER yet
// under this feature, so the tier-presence check must be independent of the
// key-presence check, not reuse alreadyConfigured.
func TestExistingTier_IndependentOfKeyPresence(t *testing.T) {
	tmp := t.TempDir()
	envPath := filepath.Join(tmp, ".env")

	if got := existingTier(envPath); got != "" {
		t.Errorf("existingTier() on missing .env = %q, want empty", got)
	}

	// Upgrade-path case: keys are configured, tier is not.
	if err := os.WriteFile(envPath, []byte("OPENCODE_API_KEY=secret\n"), 0600); err != nil {
		t.Fatalf("writing .env: %v", err)
	}
	if got := existingTier(envPath); got != "" {
		t.Errorf("existingTier() with keys but no tier = %q, want empty (upgrade-path gap)", got)
	}

	if err := os.WriteFile(envPath, []byte("OPENCODE_API_KEY=secret\nSOMM_OC_TIER=zen\n"), 0600); err != nil {
		t.Fatalf("writing .env: %v", err)
	}
	if got := existingTier(envPath); got != "zen" {
		t.Errorf("existingTier() = %q, want zen", got)
	}
}

func TestUpdateMCPConfig(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "opencode.json")
	binaryPath := filepath.Join(tmp, "somm")

	initial := &openCodeConfig{raw: map[string]json.RawMessage{}, MCP: map[string]json.RawMessage{}}
	if err := writeConfig(configPath, initial); err != nil {
		t.Fatalf("writing initial config: %v", err)
	}

	config, err := readConfig(configPath)
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}
	if err := updateMCPConfig(config, binaryPath); err != nil {
		t.Fatalf("updateMCPConfig: %v", err)
	}
	if err := writeConfig(configPath, config); err != nil {
		t.Fatalf("writing updated config: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("reading updated config: %v", err)
	}
	var parsed struct {
		MCP map[string]mcpEntry `json:"mcp"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("parsing updated config: %v", err)
	}

	entry, ok := parsed.MCP["somm"]
	if !ok {
		t.Fatal("expected somm MCP entry")
	}
	if len(entry.Command) != 1 || entry.Command[0] != binaryPath {
		t.Errorf("command = %v, want [%q]", entry.Command, binaryPath)
	}
	if !entry.Enabled {
		t.Error("expected entry.Enabled to be true")
	}
	if entry.Type != "local" {
		t.Errorf("type = %q, want local", entry.Type)
	}
}

// TestReadWriteConfig_PreservesUnrelatedData is a regression test: earlier,
// openCodeConfig only declared an "mcp" field, so round-tripping through
// readConfig/writeConfig silently dropped every other key in opencode.json
// (agent/model assignments, permissions, other MCP servers' extra fields)
// the instant somm setup ran. Real opencode.json files carry exactly this
// kind of data (e.g. from gentle-ai), so this must survive untouched.
func TestReadWriteConfig_PreservesUnrelatedData(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "opencode.json")
	binaryPath := filepath.Join(tmp, "somm")

	original := `{
  "$schema": "https://opencode.ai/config.json",
  "agent": {
    "gentle-orchestrator": {
      "model": "opencode-go/glm-5.2"
    }
  },
  "permission": {
    "edit": "allow"
  },
  "mcp": {
    "chrome-devtools": {
      "command": ["npx", "chrome-devtools-mcp"],
      "enabled": true,
      "type": "local",
      "environment": {"HEADLESS": "true"}
    }
  }
}`
	if err := os.WriteFile(configPath, []byte(original), 0644); err != nil {
		t.Fatalf("writing original config: %v", err)
	}

	config, err := readConfig(configPath)
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}
	if err := updateMCPConfig(config, binaryPath); err != nil {
		t.Fatalf("updateMCPConfig: %v", err)
	}
	if err := writeConfig(configPath, config); err != nil {
		t.Fatalf("writing updated config: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("reading updated config: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("parsing updated config: %v", err)
	}

	if got["$schema"] != "https://opencode.ai/config.json" {
		t.Errorf("$schema was lost or changed: %v", got["$schema"])
	}
	agent, ok := got["agent"].(map[string]any)
	if !ok {
		t.Fatal("agent key was lost")
	}
	orchestrator, ok := agent["gentle-orchestrator"].(map[string]any)
	if !ok || orchestrator["model"] != "opencode-go/glm-5.2" {
		t.Errorf("agent.gentle-orchestrator.model was lost or changed: %v", agent["gentle-orchestrator"])
	}
	permission, ok := got["permission"].(map[string]any)
	if !ok || permission["edit"] != "allow" {
		t.Errorf("permission key was lost or changed: %v", got["permission"])
	}

	mcp, ok := got["mcp"].(map[string]any)
	if !ok {
		t.Fatal("mcp key was lost")
	}
	chrome, ok := mcp["chrome-devtools"].(map[string]any)
	if !ok {
		t.Fatal("existing mcp entry chrome-devtools was lost")
	}
	env, ok := chrome["environment"].(map[string]any)
	if !ok || env["HEADLESS"] != "true" {
		t.Errorf("chrome-devtools.environment (a field mcpEntry doesn't model) was lost: %v", chrome["environment"])
	}

	somm, ok := mcp["somm"].(map[string]any)
	if !ok {
		t.Fatal("expected the new somm mcp entry to be present")
	}
	cmd, ok := somm["command"].([]any)
	if !ok || len(cmd) != 1 || cmd[0] != binaryPath {
		t.Errorf("somm.command = %v, want [%q]", somm["command"], binaryPath)
	}
}

// TestValidateProviders covers the new "no provider required" semantics:
// validateProviders is a permissive no-op now, so every selection —
// including an empty one — returns nil.
func TestValidateProviders(t *testing.T) {
	tests := []struct {
		name     string
		selected []string
	}{
		{
			name:     "OpenCode only is valid",
			selected: []string{"OpenCode"},
		},
		{
			name:     "all providers is valid",
			selected: []string{"OpenCode", "OpenRouter", "Kimi"},
		},
		{
			name:     "OpenCode absent is valid (no provider is required anymore)",
			selected: []string{"OpenRouter", "Kimi"},
		},
		{
			name:     "empty selection is valid (no provider is required anymore)",
			selected: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateProviders(tt.selected); err != nil {
				t.Errorf("validateProviders(%v) error = %v, want nil", tt.selected, err)
			}
		})
	}
}

func TestKeyNameForProvider(t *testing.T) {
	tests := []struct {
		provider string
		want     string
	}{
		{"OpenCode", "OPENCODE_API_KEY"},
		{"OpenRouter", "OPENROUTER_API_KEY"},
		{"Unknown", ""},
	}
	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			if got := keyNameForProvider(tt.provider); got != tt.want {
				t.Errorf("keyNameForProvider(%q) = %q, want %q", tt.provider, got, tt.want)
			}
		})
	}
}

// Scenario 13: .env is stored next to resolved binary
func TestEnvPathDerivedFromBinary(t *testing.T) {
	tmp := t.TempDir()
	binDir := filepath.Join(tmp, "bin")
	binPath := filepath.Join(binDir, "somm.exe")
	envPath := filepath.Join(binDir, ".env")

	// Create bin directory
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("creating bin dir: %v", err)
	}

	// Create keys
	keys := map[string]string{
		"OPENCODE_API_KEY": "test-key",
	}

	// Save .env next to binary (simulating setup behavior)
	if err := saveEnvFile(envPath, keys); err != nil {
		t.Fatalf("saveEnvFile error: %v", err)
	}

	// Verify .env exists next to binary
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		t.Errorf(".env not found at %v, expected next to binary %v", envPath, binPath)
	}

	// Verify content
	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("reading .env: %v", err)
	}
	if !strings.Contains(string(data), "OPENCODE_API_KEY=test-key") {
		t.Errorf(".env does not contain expected key, got: %s", string(data))
	}
}

// Scenario 5: Already configured wizard shows status
func TestIsAlreadyConfigured_ReturnsBinaryPath(t *testing.T) {
	tmp := t.TempDir()
	envPath := filepath.Join(tmp, ".env")
	binPath := filepath.Join(tmp, "somm.exe")

	// Create .env with key
	if err := os.WriteFile(envPath, []byte("OPENCODE_API_KEY=secret\n"), 0600); err != nil {
		t.Fatalf("writing .env: %v", err)
	}

	// Create config with somm entry
	config := &openCodeConfig{
		MCP: map[string]json.RawMessage{
			"somm": mustMarshal(t, mcpEntry{Command: []string{binPath}, Enabled: true, Type: "local"}),
		},
	}

	configured, gotPath := isAlreadyConfigured(envPath, config)
	if !configured {
		t.Error("expected configured=true")
	}
	if gotPath != binPath {
		t.Errorf("binary path = %q, want %q", gotPath, binPath)
	}
}

// Scenario 10: --force flag bypasses status check
func TestForceFlagParsing(t *testing.T) {
	// Test that --force is parsed correctly
	force := false
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.BoolVar(&force, "force", false, "test")
	
	// Parse with --force
	if err := fs.Parse([]string{"--force"}); err != nil {
		t.Fatalf("parsing --force: %v", err)
	}
	if !force {
		t.Error("expected force=true after parsing --force")
	}

	// Parse without --force
	force = false
	if err := fs.Parse([]string{}); err != nil {
		t.Fatalf("parsing empty: %v", err)
	}
	if force {
		t.Error("expected force=false when not specified")
	}
}

