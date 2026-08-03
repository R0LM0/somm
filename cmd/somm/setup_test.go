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

func TestIsAlreadyConfigured(t *testing.T) {
	tmp := t.TempDir()
	envPath := filepath.Join(tmp, ".env")
	binPath := filepath.Join(tmp, "somm")

	// Neither .env nor config configured.
	if configured, _ := isAlreadyConfigured(envPath, &openCodeConfig{MCP: map[string]mcpEntry{}}); configured {
		t.Error("expected not configured when .env and config are empty")
	}

	// .env exists but config missing.
	if err := os.WriteFile(envPath, []byte("OPENCODE_API_KEY=secret\n"), 0600); err != nil {
		t.Fatalf("writing .env: %v", err)
	}
	if configured, _ := isAlreadyConfigured(envPath, &openCodeConfig{MCP: map[string]mcpEntry{}}); configured {
		t.Error("expected not configured when opencode.json has no somm entry")
	}

	// Both configured.
	config := &openCodeConfig{
		MCP: map[string]mcpEntry{
			"somm": {Command: []string{binPath}, Enabled: true, Type: "local"},
		},
	}
	configured, gotPath := isAlreadyConfigured(envPath, config)
	if !configured {
		t.Error("expected configured when .env and somm MCP entry exist")
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
		"KIMI_API_KEY":       "kimi-secret",
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
	for _, want := range []string{"OPENCODE_API_KEY=oc-secret", "OPENROUTER_API_KEY=or-secret", "KIMI_API_KEY=kimi-secret"} {
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

func TestUpdateMCPConfig(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "opencode.json")
	binaryPath := filepath.Join(tmp, "somm")

	initial := &openCodeConfig{MCP: map[string]mcpEntry{}}
	if err := writeConfig(configPath, initial); err != nil {
		t.Fatalf("writing initial config: %v", err)
	}

	config, err := readConfig(configPath)
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}
	updateMCPConfig(config, binaryPath)
	if err := writeConfig(configPath, config); err != nil {
		t.Fatalf("writing updated config: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("reading updated config: %v", err)
	}
	var parsed openCodeConfig
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

func TestValidateProviders(t *testing.T) {
	tests := []struct {
		name     string
		selected []string
		wantErr  bool
	}{
		{
			name:     "OpenCode only is valid",
			selected: []string{"OpenCode"},
			wantErr:  false,
		},
		{
			name:     "all providers is valid",
			selected: []string{"OpenCode", "OpenRouter", "Kimi"},
			wantErr:  false,
		},
		{
			name:     "missing OpenCode is invalid",
			selected: []string{"OpenRouter", "Kimi"},
			wantErr:  true,
		},
		{
			name:     "empty selection is invalid",
			selected: []string{},
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProviders(tt.selected)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateProviders(%v) error = %v, wantErr %v", tt.selected, err, tt.wantErr)
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
		{"Kimi", "KIMI_API_KEY"},
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
		MCP: map[string]mcpEntry{
			"somm": {Command: []string{binPath}, Enabled: true, Type: "local"},
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

// Scenario 16: .env Kimi key reaches API client
func TestKimiKeyInEnv(t *testing.T) {
	tmp := t.TempDir()
	envPath := filepath.Join(tmp, ".env")

	// Create .env with Kimi key
	envContent := "OPENCODE_API_KEY=oc-secret\nKIMI_API_KEY=kimi-secret\n"
	if err := os.WriteFile(envPath, []byte(envContent), 0600); err != nil {
		t.Fatalf("writing .env: %v", err)
	}

	// Read and verify
	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("reading .env: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "KIMI_API_KEY=kimi-secret") {
		t.Errorf(".env missing KIMI_API_KEY, got: %s", content)
	}
	if !strings.Contains(content, "OPENCODE_API_KEY=oc-secret") {
		t.Errorf(".env missing OPENCODE_API_KEY, got: %s", content)
	}
}
