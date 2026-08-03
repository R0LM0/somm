package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// testModel builds a Model wired to no-op fakes for every I/O boundary
// (network/download, .env, opencode.json, PowerShell profile), so state
// transitions can be exercised without touching the real filesystem or
// network.
func testModel(t *testing.T) Model {
	t.Helper()
	tmp := t.TempDir()
	binaryPath := filepath.Join(tmp, "somm"+exeSuffix())
	if err := os.WriteFile(binaryPath, []byte("fake"), 0755); err != nil {
		t.Fatalf("writing fake binary: %v", err)
	}

	m := newModel(modelInit{
		configPath:        filepath.Join(tmp, "opencode.json"),
		config:            &openCodeConfig{raw: map[string]json.RawMessage{}, MCP: map[string]json.RawMessage{}},
		binaryPath:        binaryPath,
		envPath:           filepath.Join(tmp, ".env"),
		alreadyConfigured: false,
		updateAvailable:   false,
		currentVersion:    "test",
	})
	m.updateBinaryFn = func(string) error { return nil }
	m.saveEnvFileFn = func(string, map[string]string) error { return nil }
	m.writeConfigFn = func(string, *openCodeConfig) error { return nil }
	m.createAliasFn = func() error { return nil }
	return m
}

func sendKey(m Model, s string) Model {
	var msg tea.KeyMsg
	switch s {
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEscape}
	case "space":
		msg = tea.KeyMsg{Type: tea.KeySpace}
	case "tab":
		msg = tea.KeyMsg{Type: tea.KeyTab}
	case "left":
		msg = tea.KeyMsg{Type: tea.KeyLeft}
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
	next, _ := m.Update(msg)
	return next.(Model)
}

func TestMenuNavigationWraps(t *testing.T) {
	m := testModel(t)
	if m.menuCursor != 0 {
		t.Fatalf("initial cursor = %d, want 0", m.menuCursor)
	}

	m = sendKey(m, "k") // up from the first entry wraps to the last
	if m.menuCursor != len(m.menu)-1 {
		t.Fatalf("cursor after k = %d, want %d", m.menuCursor, len(m.menu)-1)
	}

	m = sendKey(m, "j")
	if m.menuCursor != 0 {
		t.Fatalf("cursor after j = %d, want 0", m.menuCursor)
	}
}

func TestMenuConfigureOpensConfirm(t *testing.T) {
	m := testModel(t)
	m = sendKey(m, "enter") // menuCursor 0 = actConfigure

	if m.screen != scrConfirm {
		t.Fatalf("screen = %v, want scrConfirm", m.screen)
	}
	if m.confirm.kind != confirmAdd {
		t.Fatalf("confirm.kind = %v, want confirmAdd", m.confirm.kind)
	}
	if !m.confirm.yes {
		t.Fatalf("confirm.yes = false, want true (affirmative highlighted by default)")
	}
}

func TestConfirmDeclineReturnsToMenu(t *testing.T) {
	m := testModel(t)
	m = sendKey(m, "enter") // open the "add to OpenCode" confirm
	m = sendKey(m, "left")  // toggle to "No"
	if m.confirm.yes {
		t.Fatalf("confirm.yes = true after toggling left, want false")
	}

	m = sendKey(m, "enter") // confirm the highlighted "No"
	if m.screen != scrMenu {
		t.Fatalf("screen = %v, want scrMenu after declining", m.screen)
	}
}

func TestConfirmAddAcceptSkipsRelocationWhenAlreadyCorrect(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("GOPATH", tmp)
	binDir := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	binaryPath := filepath.Join(binDir, "somm"+exeSuffix())
	if err := os.WriteFile(binaryPath, []byte("fake"), 0755); err != nil {
		t.Fatal(err)
	}

	m := testModel(t)
	m.binaryPath = binaryPath

	m = sendKey(m, "enter") // open confirm add
	m = sendKey(m, "enter") // accept (default "yes")

	if m.screen != scrProviders {
		t.Fatalf("screen = %v, want scrProviders", m.screen)
	}
	if len(m.providers) == 0 || !m.providers[0].selected || !m.providers[0].required {
		t.Fatalf("providers not initialized as expected: %+v", m.providers)
	}
}

func TestProvidersToggleAndSubmit(t *testing.T) {
	m := testModel(t)
	m = m.enterProviders()

	m = sendKey(m, "j")     // move cursor to OpenRouter
	m = sendKey(m, "space") // toggle it on
	if !m.providers[1].selected {
		t.Fatalf("OpenRouter not selected after space")
	}

	m = sendKey(m, "enter")
	if m.screen != scrProviderKeyInput {
		t.Fatalf("screen = %v, want scrProviderKeyInput", m.screen)
	}
	if len(m.keyFields) != 2 {
		t.Fatalf("keyFields = %d, want 2 (OpenCode + OpenRouter)", len(m.keyFields))
	}
}

func TestProvidersRequiredCannotBeToggled(t *testing.T) {
	m := testModel(t)
	m = m.enterProviders() // cursor starts on OpenCode (index 0, required)

	m = sendKey(m, "space")
	if !m.providers[0].selected {
		t.Fatalf("required OpenCode provider got deselected by space")
	}
}

func TestKeyInputRejectsEmptyOpenCodeKey(t *testing.T) {
	m := testModel(t)
	m = m.enterProviders()
	m = sendKey(m, "enter") // only OpenCode selected -> a single key field

	m = sendKey(m, "enter") // try to submit an empty required key
	if m.screen != scrProviderKeyInput {
		t.Fatalf("screen = %v, want scrProviderKeyInput (must stay put on empty required key)", m.screen)
	}
	if m.keyErr == "" {
		t.Fatalf("expected keyErr to be set for an empty required key")
	}
}

func TestConfigDoneMsgSuccessMarksConfigured(t *testing.T) {
	m := testModel(t)
	m.screen = scrConfigProgress

	next, _ := m.Update(configDoneMsg{steps: []string{"✓ .env guardado"}})
	got := next.(Model)

	if got.screen != scrConfigResult {
		t.Fatalf("screen = %v, want scrConfigResult", got.screen)
	}
	if !got.alreadyConfigured {
		t.Fatalf("alreadyConfigured = false, want true after a successful save")
	}
	if got.err != nil {
		t.Fatalf("err = %v, want nil", got.err)
	}
}

func TestUpdateDoneMsgFailureKeepsUpdateAvailable(t *testing.T) {
	m := testModel(t)
	m.updateAvailable = true
	m.latestVersion = "v9.9.9"
	m.screen = scrUpdateProgress

	wantErr := errors.New("network down")
	next, _ := m.Update(updateDoneMsg{err: wantErr, latestVersion: "v9.9.9"})
	got := next.(Model)

	if got.screen != scrUpdateResult {
		t.Fatalf("screen = %v, want scrUpdateResult", got.screen)
	}
	if !got.updateAvailable {
		t.Fatalf("updateAvailable = false, want true (a failed update is still pending)")
	}
	if len(got.resultLines) == 0 {
		t.Fatalf("expected resultLines to describe the failure")
	}
}

var ansiRe = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripANSI(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

func TestViewMenuContainsExpectedEntries(t *testing.T) {
	m := testModel(t)
	out := stripANSI(m.View())

	for _, want := range []string{
		"Somm Setup Wizard",
		"Configurar somm en OpenCode",
		"Buscar actualizaciones",
		"Salir",
		helpNavigate,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("menu view missing %q\ngot:\n%s", want, out)
		}
	}
}
