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

	binary := filepath.Join(t.TempDir(), "server"+exeSuffix())
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = filepath.Join(repoRoot, "cmd", "server")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		t.Skipf("skipping: failed to build server binary: %v", err)
	}

	cmd := exec.Command(binary, "-opencode-api-key", "test-key")
	cmd.Dir = repoRoot

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
	if err := stdin.Close(); err != nil {
		t.Fatalf("close stdin: %v", err)
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
	if !strings.Contains(body, `"model-advisor"`) {
		t.Fatalf("initialize response missing server name model-advisor: %s", body)
	}
}

// TestServerRequiresOpenCodeKey verifies that the binary exits with an error
// when the required -opencode-api-key flag is missing.
func TestServerRequiresOpenCodeKey(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess smoke test in short mode")
	}

	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("finding repo root: %v", err)
	}

	binary := filepath.Join(t.TempDir(), "server"+exeSuffix())
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = filepath.Join(repoRoot, "cmd", "server")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		t.Skipf("skipping: failed to build server binary: %v", err)
	}

	cmd := exec.Command(binary)
	cmd.Dir = t.TempDir() // Run in temp dir without .env
	cmd.Stderr = os.Stderr
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
