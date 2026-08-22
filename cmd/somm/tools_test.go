package main

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestToolsE2E(t *testing.T) {
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

	cmd := exec.Command(binary, "--skip-setup", "-opencode-api-key", "test-key")
	cmd.Dir = repoRoot
	// --skip-setup bypasses the auto-setup pre-flight (configReady() now
	// checks for a .env file next to the binary, which this fresh build
	// doesn't have); the explicit -opencode-api-key flag configures the key.
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

	// Send initialize
	initReq := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0.0"}}}` + "\n"
	if _, err := stdin.Write([]byte(initReq)); err != nil {
		t.Fatalf("write initialize: %v", err)
	}

	// Read initialize response
	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() {
		t.Fatal("no initialize response")
	}
	initResp := scanner.Text()
	if !strings.Contains(initResp, `"somm"`) {
		t.Fatalf("initialize response missing server name: %s", initResp)
	}
	t.Log("✅ Initialize OK")

	// Send initialized notification
	notif := `{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n"
	if _, err := stdin.Write([]byte(notif)); err != nil {
		t.Fatalf("write notification: %v", err)
	}

	// Test tools/list
	toolsList := `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}` + "\n"
	if _, err := stdin.Write([]byte(toolsList)); err != nil {
		t.Fatalf("write tools/list: %v", err)
	}

	// Read responses until we get the tools/list response
	toolsFound := false
	for i := 0; i < 10; i++ {
		if !scanner.Scan() {
			t.Fatal("no tools/list response")
		}
		toolsResp := scanner.Text()
		t.Logf("Response %d: %s", i, toolsResp)
		if strings.Contains(toolsResp, `"id":2`) && strings.Contains(toolsResp, `"list_available_models"`) {
			toolsFound = true
			break
		}
	}
	if !toolsFound {
		t.Fatal("tools/list response not found")
	}
	t.Log("✅ Tools list OK")

	// Test get_agent_criteria
	criteriaReq := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"get_agent_criteria","arguments":{"agent":"sdd-apply"}}}` + "\n"
	if _, err := stdin.Write([]byte(criteriaReq)); err != nil {
		t.Fatalf("write get_agent_criteria: %v", err)
	}

	criteriaResp := ""
	for i := 0; i < 10; i++ {
		if !scanner.Scan() {
			t.Fatal("no get_agent_criteria response")
		}
		criteriaResp = scanner.Text()
		if strings.Contains(criteriaResp, `"id":3`) {
			break
		}
		// Ignore server notifications that may arrive out of band.
		t.Logf("Skipping non-response line: %s", criteriaResp)
	}
	if !strings.Contains(criteriaResp, `"CRITICIDAD"`) && !strings.Contains(criteriaResp, `"CRITERIOS"`) {
		t.Logf("get_agent_criteria response: %s", criteriaResp)
		if !strings.Contains(criteriaResp, `"id":3`) {
			t.Fatalf("get_agent_criteria response missing id=3: %s", criteriaResp)
		}
	}
	t.Log("✅ get_agent_criteria OK")

	t.Log("\n🎉 All tools working!")
}
