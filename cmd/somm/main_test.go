package main

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestServerInitialize starts the built server binary, sends an MCP initialize
// request over stdin, and verifies the response contains the expected id and
// server name.
func TestServerInitialize(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess smoke test in short mode")
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

	cmd := exec.Command(binary, "-opencode-api-key", "test-key")
	cmd.Dir = repoRoot
	// Set dummy API key to bypass auto-setup check
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

	initReq := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test-client","version":"1.0.0"}}}` + "\n"
	if _, err := stdin.Write([]byte(initReq)); err != nil {
		t.Fatalf("write initialize request: %v", err)
	}

	done := make(chan struct{})
	var resp bytes.Buffer
	go func() {
		defer close(done)
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			resp.Write(scanner.Bytes())
			resp.WriteByte('\n')
			break
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for initialize response")
	}

	body := resp.String()
	if !strings.Contains(body, `"id":1`) {
		t.Fatalf("initialize response missing id=1: %s", body)
	}
	if !strings.Contains(body, `"somm"`) {
		t.Fatalf("initialize response missing server name somm: %s", body)
	}
}

// TestServerRequiresOpenCodeKey verifies that the binary exits with the legacy
// error when the required key is missing and --skip-setup is passed.
func TestServerRequiresOpenCodeKey(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess smoke test in short mode")
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

	var stderr bytes.Buffer
	cmd := exec.Command(binary, "--skip-setup")
	cmd.Dir = t.TempDir() // Run in temp dir without .env
	cmd.Stderr = &stderr
	cmd.Env = append(os.Environ(), "OPENCODE_API_KEY=", "OPENROUTER_API_KEY=")

	err = cmd.Run()
	if err == nil {
		t.Fatal("expected server to fail without -opencode-api-key, but it succeeded")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected *exec.ExitError, got %T: %v", err, err)
	}
	if exitErr.ExitCode() == 0 {
		t.Fatalf("expected non-zero exit code, got %d", exitErr.ExitCode())
	}
	if !strings.Contains(stderr.String(), "-opencode-api-key is required") {
		t.Fatalf("expected legacy error message, got stderr: %s", stderr.String())
	}
}

// TestServerFailsFatalOnUnrecognizedTier verifies run() fails fatal, in the
// same fail-loud style as -opencode-api-key required, when SOMM_OC_TIER
// holds a value outside the go|zen domain.
func TestServerFailsFatalOnUnrecognizedTier(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess smoke test in short mode")
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

	var stderr bytes.Buffer
	cmd := exec.Command(binary, "--skip-setup", "-opencode-api-key", "test-key")
	cmd.Dir = t.TempDir() // Run in temp dir without .env
	cmd.Stderr = &stderr
	cmd.Env = append(os.Environ(), "OPENCODE_API_KEY=test-key", "OPENROUTER_API_KEY=", "SOMM_OC_TIER=bogus")

	err = cmd.Run()
	if err == nil {
		t.Fatal("expected server to fail with an unrecognized SOMM_OC_TIER, but it succeeded")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected *exec.ExitError, got %T: %v", err, err)
	}
	if exitErr.ExitCode() == 0 {
		t.Fatalf("expected non-zero exit code, got %d", exitErr.ExitCode())
	}
	if !strings.Contains(stderr.String(), "SOMM_OC_TIER") {
		t.Fatalf("expected an error message naming SOMM_OC_TIER, got stderr: %s", stderr.String())
	}
}

func TestLoadEnvFile(t *testing.T) {
	tmp := t.TempDir()
	envPath := filepath.Join(tmp, ".env")

	if err := os.WriteFile(envPath, []byte("OPENCODE_API_KEY=from-env-file\nOPENROUTER_API_KEY=or-from-file\n"), 0644); err != nil {
		t.Fatalf("writing .env: %v", err)
	}

	t.Setenv("OPENCODE_API_KEY", "")
	t.Setenv("OPENROUTER_API_KEY", "")

	loadEnvFile(envPath)

	if got := os.Getenv("OPENCODE_API_KEY"); got != "from-env-file" {
		t.Errorf("OPENCODE_API_KEY = %q, want from-env-file", got)
	}
	if got := os.Getenv("OPENROUTER_API_KEY"); got != "or-from-file" {
		t.Errorf("OPENROUTER_API_KEY = %q, want or-from-file", got)
	}
}

func TestLoadEnvFile_DoesNotOverrideExisting(t *testing.T) {
	tmp := t.TempDir()
	envPath := filepath.Join(tmp, ".env")

	if err := os.WriteFile(envPath, []byte("OPENCODE_API_KEY=from-env-file\n"), 0644); err != nil {
		t.Fatalf("writing .env: %v", err)
	}

	t.Setenv("OPENCODE_API_KEY", "existing")

	loadEnvFile(envPath)

	if got := os.Getenv("OPENCODE_API_KEY"); got != "existing" {
		t.Errorf("OPENCODE_API_KEY = %q, want existing", got)
	}
}

func TestConfigReady(t *testing.T) {
	tests := []struct {
		name    string
		envValue string
		want    bool
	}{
		{"key present", "secret", true},
		{"key missing", "", false},
		{"key whitespace only", "   ", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("OPENCODE_API_KEY", tt.envValue)
			if got := configReady(); got != tt.want {
				t.Errorf("configReady() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsTerminal(t *testing.T) {
	// Stdin in tests is not a terminal.
	if isTerminal() {
		t.Error("isTerminal() = true, want false in test environment")
	}
}

func TestSkipSetupFlagDetected(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{"no flag", []string{"somm"}, false},
		{"with flag", []string{"somm", "--skip-setup"}, true},
		{"flag with value after", []string{"somm", "--skip-setup", "arg"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := skipSetupFlag(tt.args); got != tt.want {
				t.Errorf("skipSetupFlag(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestMaybeRunSetup(t *testing.T) {
	t.Run("skip setup returns true", func(t *testing.T) {
		if !maybeRunSetup(true) {
			t.Error("maybeRunSetup(true) = false, want true")
		}
	})

	t.Run("config ready returns true", func(t *testing.T) {
		t.Setenv("OPENCODE_API_KEY", "ready")
		if !maybeRunSetup(false) {
			t.Error("maybeRunSetup(false) with config ready = false, want true")
		}
	})

	t.Run("missing key in non-terminal returns false", func(t *testing.T) {
		t.Setenv("OPENCODE_API_KEY", "")
		if maybeRunSetup(false) {
			t.Error("maybeRunSetup(false) with missing key in non-terminal = true, want false")
		}
	})
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}

func exeSuffix() string {
	if os.PathSeparator == '\\' {
		return ".exe"
	}
	return ""
}
