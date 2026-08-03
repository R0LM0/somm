package main

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestE2E_NoArgsNoEnv_ShowsError verifies that running somm without
// arguments and without .env shows a clear error message (not the wizard,
// because stdin is not a terminal).
func TestE2E_NoArgsNoEnv_ShowsError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("finding repo root: %v", err)
	}

	binary := filepath.Join(t.TempDir(), "somm"+exeSuffix())
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = filepath.Join(repoRoot, "cmd", "somm")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		t.Skipf("skipping: failed to build somm binary: %v", err)
	}

	// Run without args and without .env
	cmd := exec.Command(binary)
	cmd.Dir = t.TempDir() // Empty dir, no .env
	// Clear all API keys
	cmd.Env = append(os.Environ(),
		"OPENCODE_API_KEY=",
		"OPENROUTER_API_KEY=",
		"KIMI_API_KEY=",
	)

	var stderr strings.Builder
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err == nil {
		t.Fatal("expected command to fail")
	}

	errMsg := stderr.String()
	// Should show error about missing key
	if !strings.Contains(errMsg, "OPENCODE_API_KEY") && !strings.Contains(errMsg, "opencode-api-key") {
		t.Errorf("expected error about missing API key, got:\n%s", errMsg)
	}
	// Should hint about setup
	if !strings.Contains(errMsg, "somm setup") && !strings.Contains(errMsg, "--skip-setup") {
		t.Errorf("expected hint about setup or --skip-setup, got:\n%s", errMsg)
	}
}

// TestE2E_NoArgsWithEnv_ServerStarts verifies that running somm without
// arguments but with .env configured starts the MCP server.
func TestE2E_NoArgsWithEnv_ServerStarts(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("finding repo root: %v", err)
	}

	binary := filepath.Join(t.TempDir(), "somm"+exeSuffix())
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = filepath.Join(repoRoot, "cmd", "somm")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		t.Skipf("skipping: failed to build somm binary: %v", err)
	}

	// Create .env with API key
	envPath := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(envPath, []byte("OPENCODE_API_KEY=test-key\n"), 0600); err != nil {
		t.Fatalf("writing .env: %v", err)
	}

	// Run without args but with API key
	cmd := exec.Command(binary)
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), "OPENCODE_API_KEY=test-key")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	// Send initialize request
	initReq := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0.0"}}}` + "\n"
	if _, err := stdin.Write([]byte(initReq)); err != nil {
		t.Fatalf("write initialize: %v", err)
	}

	// Read response
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	if !scanner.Scan() {
		t.Fatal("no initialize response - server did not start")
	}
	initResp := scanner.Text()
	if !strings.Contains(initResp, `"somm"`) {
		t.Fatalf("initialize response missing server name: %s", initResp)
	}
	t.Log("✅ Server started successfully with API key")
}

// TestE2E_Help_ShowsUsage verifies that --help shows usage information
func TestE2E_Help_ShowsUsage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("finding repo root: %v", err)
	}

	binary := filepath.Join(t.TempDir(), "somm"+exeSuffix())
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = filepath.Join(repoRoot, "cmd", "somm")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		t.Skipf("skipping: failed to build somm binary: %v", err)
	}

	tests := []struct {
		name string
		args []string
	}{
		{"--help", []string{"--help"}},
		{"-help", []string{"-help"}},
		{"help", []string{"help"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(binary, tt.args...)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("command failed: %v\noutput: %s", err, output)
			}

			out := string(output)
			if !strings.Contains(out, "Somm") || !strings.Contains(out, "Usage") {
				t.Errorf("expected help output, got:\n%s", out)
			}
			// Should NOT show setup wizard
			if strings.Contains(out, "Setup Wizard") {
				t.Errorf("help should not trigger setup wizard, got:\n%s", out)
			}
		})
	}
}

// TestE2E_Version_ShowsVersion verifies that --version shows version info
func TestE2E_Version_ShowsVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("finding repo root: %v", err)
	}

	binary := filepath.Join(t.TempDir(), "somm"+exeSuffix())
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = filepath.Join(repoRoot, "cmd", "somm")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		t.Skipf("skipping: failed to build somm binary: %v", err)
	}

	tests := []struct {
		name string
		args []string
	}{
		{"--version", []string{"--version"}},
		{"-version", []string{"-version"}},
		{"version", []string{"version"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(binary, tt.args...)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("command failed: %v\noutput: %s", err, output)
			}

			out := string(output)
			if !strings.Contains(out, "somm") || !strings.Contains(out, "dev") {
				t.Errorf("expected version output, got:\n%s", out)
			}
			// Should NOT show setup wizard
			if strings.Contains(out, "Setup Wizard") {
				t.Errorf("version should not trigger setup wizard, got:\n%s", out)
			}
		})
	}
}

// TestE2E_SkipSetup_WithNoKey_ShowsError verifies that --skip-setup with
// no API key shows a clear error message
func TestE2E_SkipSetup_WithNoKey_ShowsError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("finding repo root: %v", err)
	}

	binary := filepath.Join(t.TempDir(), "somm"+exeSuffix())
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = filepath.Join(repoRoot, "cmd", "somm")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		t.Skipf("skipping: failed to build somm binary: %v", err)
	}

	var stderr strings.Builder
	cmd := exec.Command(binary, "--skip-setup")
	cmd.Dir = t.TempDir()
	cmd.Stderr = &stderr
	cmd.Env = append(os.Environ(), "OPENCODE_API_KEY=", "OPENROUTER_API_KEY=", "KIMI_API_KEY=")

	err = cmd.Run()
	if err == nil {
		t.Fatal("expected command to fail")
	}

	errMsg := stderr.String()
	// Should show error about missing key (either message is acceptable)
	if !strings.Contains(errMsg, "OPENCODE_API_KEY") && !strings.Contains(errMsg, "opencode-api-key") {
		t.Errorf("expected error about missing API key, got:\n%s", errMsg)
	}
}

// TestE2E_SommSetup_WithNoKey_ShowsWizard verifies that "somm setup"
// shows the setup wizard
func TestE2E_SommSetup_WithNoKey_ShowsWizard(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("finding repo root: %v", err)
	}

	binary := filepath.Join(t.TempDir(), "somm"+exeSuffix())
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = filepath.Join(repoRoot, "cmd", "somm")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		t.Skipf("skipping: failed to build somm binary: %v", err)
	}

	// Create mock opencode.json in temp home directory
	tmpHome := t.TempDir()
	configDir := filepath.Join(tmpHome, ".config", "opencode")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("creating config dir: %v", err)
	}
	mockConfig := filepath.Join(configDir, "opencode.json")
	if err := os.WriteFile(mockConfig, []byte(`{"mcp":{}}`), 0644); err != nil {
		t.Fatalf("writing mock config: %v", err)
	}

	// Override HOME to use temp directory
	cmd := exec.Command(binary, "setup")
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(),
		"OPENCODE_API_KEY=",
		"HOME="+tmpHome,
		"USERPROFILE="+tmpHome,
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	// Read output
	done := make(chan struct{})
	var output strings.Builder
	go func() {
		defer close(done)
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			output.WriteString(scanner.Text())
			output.WriteByte('\n')
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}

	out := output.String()
	if !strings.Contains(out, "Somm Setup Wizard") {
		t.Errorf("expected setup wizard, got:\n%s", out)
	}
}
