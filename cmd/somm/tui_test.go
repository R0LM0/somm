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
	if len(m.providers) != 1 || !m.providers[0].selected {
		t.Fatalf("providers not initialized as expected: %+v", m.providers)
	}
}

func TestProvidersToggleAndSubmit(t *testing.T) {
	m := testModel(t)
	m = m.enterProviders() // cursor starts on OpenRouter, the only entry

	// OpenRouter starts selected by default, so a full toggle round-trip
	// (off, then back on) must leave it selected again.
	m = sendKey(m, "space") // toggle it off
	if m.providers[0].selected {
		t.Fatalf("OpenRouter still selected after space")
	}
	m = sendKey(m, "space") // toggle it back on
	if !m.providers[0].selected {
		t.Fatalf("OpenRouter not selected after toggling back on")
	}

	m = sendKey(m, "enter")
	if m.screen != scrProviderKeyInput {
		t.Fatalf("screen = %v, want scrProviderKeyInput", m.screen)
	}
	if len(m.keyFields) != 1 {
		t.Fatalf("keyFields = %d, want 1 (OpenRouter only — OpenCode isn't offered in the wizard, it's auto-detected)", len(m.keyFields))
	}
}

// TestProvidersAllToggleable replaces the old
// TestProvidersRequiredCannotBeToggled: no provider is required, so
// OpenRouter (the only entry offered in the wizard) must toggle freely.
func TestProvidersAllToggleable(t *testing.T) {
	m := testModel(t)
	m = m.enterProviders()

	m = sendKey(m, "space")
	if m.providers[0].selected {
		t.Fatalf("OpenRouter still selected after space, want toggled off")
	}
}

// TestKeyInputAcceptsEmptyOpenRouterKey replaces the old
// TestKeyInputRejectsEmptyOpenCodeKey: with OpenCode no longer offered in
// the wizard (auto-detected via discovery, no prompt needed), OpenRouter is
// the only field left, and it has always accepted an empty value.
func TestKeyInputAcceptsEmptyOpenRouterKey(t *testing.T) {
	m := testModel(t)
	m = m.enterProviders()
	m = sendKey(m, "enter") // OpenRouter selected by default -> its key field

	m = sendKey(m, "enter") // submit an empty OpenRouter key
	if m.keyErr != "" {
		t.Fatalf("keyErr = %q, want empty (OpenRouter key is optional)", m.keyErr)
	}
	if m.screen != scrTier {
		t.Fatalf("screen = %v, want scrTier (advanced past the only, now-empty, key field)", m.screen)
	}
}

// TestProvidersNoneSelectedAdvancesToTier covers the edge case this feature
// newly exposes: deselecting every provider and pressing enter must not
// leave the user stuck on scrProviders (there are no key fields to collect,
// so the flow advances straight to whatever key collection would have
// advanced to next).
func TestProvidersNoneSelectedAdvancesToTier(t *testing.T) {
	m := testModel(t)
	m = m.enterProviders()

	m = sendKey(m, "space") // deselect OpenRouter (the only entry)

	m = sendKey(m, "enter")
	if m.screen != scrTier {
		t.Fatalf("screen = %v, want scrTier (zero providers selected)", m.screen)
	}
	if len(m.keyFields) != 0 {
		t.Fatalf("keyFields = %d, want 0", len(m.keyFields))
	}
	if len(m.tiers) != 2 {
		t.Fatalf("tiers = %v, want 2 entries (go, zen)", m.tiers)
	}
}

// TestProvidersNoneSelectedSkipsTierScreenWhenPersisted covers the same
// zero-providers edge case when a tier is already persisted and --force was
// not passed: the flow must skip straight to scrConfigProgress, exactly
// like the last-key-field case does.
func TestProvidersNoneSelectedSkipsTierScreenWhenPersisted(t *testing.T) {
	m := testModel(t)
	m.presetTier = "go"
	m.skipTierScreen = true
	m = m.enterProviders()

	m = sendKey(m, "space") // deselect OpenRouter (the only entry)

	m = sendKey(m, "enter")
	if m.screen != scrConfigProgress {
		t.Fatalf("screen = %v, want scrConfigProgress (tier already persisted, zero providers selected)", m.screen)
	}
	if got := m.keys["SOMM_OC_TIER"]; got != "go" {
		t.Fatalf("keys[SOMM_OC_TIER] = %q, want %q", got, "go")
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

// submitAllKeyFields types "secret" into every open key field and presses
// enter, advancing past provider-key collection entirely.
func submitAllKeyFields(m Model) Model {
	for range m.keyFields {
		m = sendKey(m, "secret")
		m = sendKey(m, "enter")
	}
	return m
}

func TestKeyInputEntersTierScreenAfterLastField(t *testing.T) {
	m := testModel(t)
	m = m.enterProviders()
	m = sendKey(m, "enter") // OpenRouter selected by default -> its key field

	m = submitAllKeyFields(m)

	if m.screen != scrTier {
		t.Fatalf("screen = %v, want scrTier", m.screen)
	}
	if len(m.tiers) != 2 {
		t.Fatalf("tiers = %v, want 2 entries (go, zen)", m.tiers)
	}
}

func TestTierCursorMovesAndWraps(t *testing.T) {
	m := testModel(t)
	m = m.enterProviders()
	m = sendKey(m, "enter")
	m = submitAllKeyFields(m)

	start := m.tierCursor
	m = sendKey(m, "j")
	if m.tierCursor == start {
		t.Fatalf("tierCursor did not move after j")
	}
	m = sendKey(m, "k")
	if m.tierCursor != start {
		t.Fatalf("tierCursor after k = %d, want back to %d", m.tierCursor, start)
	}
}

func TestTierEnterSetsKeyAndAdvancesToConfigProgress(t *testing.T) {
	m := testModel(t)
	m = m.enterProviders()
	m = sendKey(m, "enter")
	m = submitAllKeyFields(m)

	want := m.tiers[m.tierCursor]
	m = sendKey(m, "enter")

	if m.screen != scrConfigProgress {
		t.Fatalf("screen = %v, want scrConfigProgress", m.screen)
	}
	if got := m.keys["SOMM_OC_TIER"]; got != want {
		t.Fatalf("keys[SOMM_OC_TIER] = %q, want %q", got, want)
	}
}

func TestTierEscReturnsToLastKeyField(t *testing.T) {
	m := testModel(t)
	m = m.enterProviders()
	m = sendKey(m, "enter")
	m = submitAllKeyFields(m)

	m = sendKey(m, "esc")
	if m.screen != scrProviderKeyInput {
		t.Fatalf("screen = %v, want scrProviderKeyInput", m.screen)
	}
	if m.keyIdx != len(m.keyFields)-1 {
		t.Fatalf("keyIdx = %d, want %d (last key field)", m.keyIdx, len(m.keyFields)-1)
	}
}

func TestTierScreenPreselectsExistingTierFromEnv(t *testing.T) {
	m := testModel(t)
	m.presetTier = "zen"

	m = m.enterProviders()
	m = sendKey(m, "enter")
	m = submitAllKeyFields(m)

	if m.screen != scrTier {
		t.Fatalf("screen = %v, want scrTier", m.screen)
	}
	if m.tiers[m.tierCursor] != "zen" {
		t.Fatalf("tierCursor selects %q, want zen", m.tiers[m.tierCursor])
	}
}

func TestTierScreenSkippedWhenAlreadyPersistedWithoutForce(t *testing.T) {
	m := testModel(t)
	m.presetTier = "go"
	m.skipTierScreen = true

	m = m.enterProviders()
	m = sendKey(m, "enter")
	m = submitAllKeyFields(m)

	if m.screen != scrConfigProgress {
		t.Fatalf("screen = %v, want scrConfigProgress (tier screen must be skipped)", m.screen)
	}
	if got := m.keys["SOMM_OC_TIER"]; got != "go" {
		t.Fatalf("keys[SOMM_OC_TIER] = %q, want %q (existing persisted tier must be preserved, not dropped)", got, "go")
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
