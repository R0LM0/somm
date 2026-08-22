package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

var sommTools = []string{
	"list_available_models",
	"get_agent_criteria",
	"get_model_benchmarks",
	"recommend_config",
	"estimate_cost",
	"compare_models",
	"validate_config",
	"export_config",
}

type screen int

const (
	scrMenu screen = iota
	scrStatus
	scrConfirm
	scrUpdateProgress
	scrUpdateResult
	scrBinaryPathInput
	scrProviders
	scrProviderKeyInput
	scrConfigProgress
	scrConfigResult
	// scrTier is appended last so no existing constant renumbers.
	scrTier
)

type menuAction int

const (
	actConfigure menuAction = iota
	actStatus
	actUpdate
	actQuit
)

type menuEntry struct {
	label  string
	action menuAction
}

type confirmKind int

const (
	confirmAdd confirmKind = iota
	confirmUpdate
	confirmReplaceBinary
	confirmCleanup
)

type confirmState struct {
	kind        confirmKind
	title       string
	affirmative string
	negative    string
	yes         bool
}

type providerOption struct {
	id       string
	label    string
	required bool
	selected bool
}

func defaultProviders() []providerOption {
	return []providerOption{
		{id: "OpenCode", label: "OpenCode Go/Zen (requerido)", required: true, selected: true},
		{id: "OpenRouter", label: "OpenRouter (opcional)"},
	}
}

type keyField struct {
	provider string
	title    string
	input    textinput.Model
}

func buildKeyFields(providers []string) []keyField {
	fields := make([]keyField, 0, len(providers))
	for _, p := range providers {
		title := ""
		switch p {
		case "OpenCode":
			title = "OpenCode API Key (requerido)"
		case "OpenRouter":
			title = "OpenRouter API Key (opcional)"
		}
		ti := textinput.New()
		ti.Placeholder = title
		ti.EchoMode = textinput.EchoPassword
		ti.EchoCharacter = '•'
		fields = append(fields, keyField{provider: p, title: title, input: ti})
	}
	return fields
}

// Model is the root Bubble Tea model driving the somm setup wizard. The
// real filesystem/network side effects are reached only through the
// *Fn fields, so tests can swap them for fakes.
type Model struct {
	configPath        string
	config            *openCodeConfig
	binaryPath        string
	envPath           string
	alreadyConfigured bool
	updateAvailable   bool
	latestVersion     string
	currentVersion    string

	updateBinaryFn func(string) error
	saveEnvFileFn  func(string, map[string]string) error
	writeConfigFn  func(string, *openCodeConfig) error
	createAliasFn  func() error

	screen     screen
	menu       []menuEntry
	menuCursor int

	spinner spinner.Model

	confirm confirmState

	pathInput textinput.Model

	providers  []providerOption
	provCursor int

	keyFields []keyField
	keyIdx    int
	keys      map[string]string
	keyErr    string

	// tiers/tierCursor drive scrTier, the OpenCode tier (go|zen) capture
	// screen inserted between "Key Prompts" and "Persistence". presetTier
	// preselects the cursor with any tier already in .env; skipTierScreen
	// skips the screen entirely when a tier is already persisted and
	// --force was not passed (setup-wizard spec, Requirement: Tier
	// Capture).
	tiers          []string
	tierCursor     int
	presetTier     string
	skipTierScreen bool

	pendingTarget string
	pendingSource string

	resultLines []string
	err         error

	quitting bool
}

type modelInit struct {
	configPath        string
	config            *openCodeConfig
	binaryPath        string
	envPath           string
	alreadyConfigured bool
	updateAvailable   bool
	latestVersion     string
	currentVersion    string
	presetTier        string
	skipTierScreen    bool
}

func newModel(init modelInit) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = titleStyle

	m := Model{
		configPath:        init.configPath,
		config:            init.config,
		binaryPath:        init.binaryPath,
		envPath:           init.envPath,
		alreadyConfigured: init.alreadyConfigured,
		updateAvailable:   init.updateAvailable,
		latestVersion:     init.latestVersion,
		currentVersion:    init.currentVersion,
		presetTier:        init.presetTier,
		skipTierScreen:    init.skipTierScreen,
		spinner:           sp,
		updateBinaryFn:    updateBinary,
		saveEnvFileFn:     saveEnvFile,
		writeConfigFn:     writeConfig,
		createAliasFn:     createPowerShellAlias,
	}
	m.menu = buildMenu(m)
	return m
}

func buildMenu(m Model) []menuEntry {
	var entries []menuEntry

	label := "Configurar somm en OpenCode"
	if m.alreadyConfigured {
		label = "Reconfigurar somm"
	}
	entries = append(entries, menuEntry{label, actConfigure})

	if m.alreadyConfigured {
		entries = append(entries, menuEntry{"Ver estado actual", actStatus})
	}

	updateLabel := "Buscar actualizaciones"
	if m.updateAvailable {
		updateLabel = fmt.Sprintf("Buscar actualizaciones (↑ %s disponible)", m.latestVersion)
	}
	entries = append(entries, menuEntry{updateLabel, actUpdate})

	entries = append(entries, menuEntry{"Salir", actQuit})
	return entries
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	nm, cmd := m.update(msg)
	return nm, cmd
}

func (m Model) update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	case spinner.TickMsg:
		if m.screen == scrUpdateProgress || m.screen == scrConfigProgress {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil
	case updateDoneMsg:
		m.screen = scrUpdateResult
		m.err = msg.err
		if msg.err != nil {
			m.resultLines = []string{
				fmt.Sprintf("No se pudo actualizar: %v", msg.err),
				"Podés descargar manualmente desde: https://github.com/R0LM0/somm/releases",
			}
		} else {
			m.updateAvailable = false
			m.resultLines = []string{
				fmt.Sprintf("%s Actualizado a %s — reiniciá \"somm setup\" para usar la nueva versión", successStyle.Render("✓"), detailStyle.Render(msg.latestVersion)),
			}
		}
		return m, nil
	case configDoneMsg:
		m.screen = scrConfigResult
		m.err = msg.err
		m.resultLines = msg.steps
		if msg.err == nil {
			m.alreadyConfigured = true
		}
		return m, nil
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlC {
		m.quitting = true
		return m, tea.Quit
	}
	// Reserve "q" as a global quit key, except while it would otherwise be
	// typed into a text field.
	if msg.String() == "q" && m.screen != scrProviderKeyInput && m.screen != scrBinaryPathInput {
		m.quitting = true
		return m, tea.Quit
	}

	switch m.screen {
	case scrMenu:
		return m.updateMenu(msg)
	case scrStatus:
		if msg.String() == "enter" || msg.String() == "esc" {
			m.screen = scrMenu
		}
		return m, nil
	case scrConfirm:
		return m.updateConfirm(msg)
	case scrUpdateProgress, scrConfigProgress:
		return m, nil
	case scrUpdateResult:
		if msg.String() == "enter" || msg.String() == "esc" {
			m.screen = scrMenu
			m.menu = buildMenu(m)
			m.menuCursor = 0
		}
		return m, nil
	case scrConfigResult:
		if msg.String() == "enter" || msg.String() == "esc" {
			m.screen = scrMenu
			m.menu = buildMenu(m)
			m.menuCursor = 0
		}
		return m, nil
	case scrBinaryPathInput:
		return m.updateBinaryPathInput(msg)
	case scrProviders:
		return m.updateProviders(msg)
	case scrProviderKeyInput:
		return m.updateProviderKeyInput(msg)
	case scrTier:
		return m.updateTier(msg)
	}
	return m, nil
}

func (m Model) updateMenu(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		m.menuCursor = (m.menuCursor + 1) % len(m.menu)
	case "k", "up":
		m.menuCursor = (m.menuCursor - 1 + len(m.menu)) % len(m.menu)
	case "enter":
		switch m.menu[m.menuCursor].action {
		case actConfigure:
			m.confirm = confirmState{
				kind:        confirmAdd,
				title:       "¿Querés agregarlo a OpenCode?",
				affirmative: "Sí, agregar",
				negative:    "No, cancelar",
				yes:         true,
			}
			m.screen = scrConfirm
		case actStatus:
			m.screen = scrStatus
		case actUpdate:
			if m.updateAvailable {
				m.confirm = confirmState{
					kind:        confirmUpdate,
					title:       fmt.Sprintf("¿Querés actualizar a la última versión (%s)?", m.latestVersion),
					affirmative: "Sí, actualizar",
					negative:    "No, mantener",
					yes:         true,
				}
				m.screen = scrConfirm
			} else {
				m.err = nil
				m.resultLines = []string{fmt.Sprintf("Ya estás en la última versión (%s)", detailStyle.Render(m.currentVersion))}
				m.screen = scrUpdateResult
			}
		case actQuit:
			m.quitting = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m Model) updateConfirm(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "left", "right", "h", "l", "tab":
		m.confirm.yes = !m.confirm.yes
		return m, nil
	case "y":
		m.confirm.yes = true
		return m.resolveConfirm()
	case "n":
		m.confirm.yes = false
		return m.resolveConfirm()
	case "enter":
		return m.resolveConfirm()
	case "esc":
		m.screen = scrMenu
		return m, nil
	}
	return m, nil
}

func (m Model) resolveConfirm() (Model, tea.Cmd) {
	yes := m.confirm.yes
	switch m.confirm.kind {
	case confirmAdd:
		if !yes {
			m.screen = scrMenu
			return m, nil
		}
		return m.afterConfirmAdd()

	case confirmUpdate:
		if !yes {
			m.screen = scrMenu
			return m, nil
		}
		m.screen = scrUpdateProgress
		return m, tea.Batch(m.spinner.Tick, doUpdateCmd(m.latestVersion, m.updateBinaryFn))

	case confirmReplaceBinary:
		if !yes {
			return m.enterProviders(), nil
		}
		if err := installBinary(m.binaryPath, m.pendingTarget); err != nil {
			m.err = err
			m.resultLines = []string{fmt.Sprintf("Error moviendo binario: %v", err)}
			m.screen = scrConfigResult
			return m, nil
		}
		old := m.binaryPath
		m.binaryPath = m.pendingTarget
		m.envPath = filepath.Join(filepath.Dir(m.binaryPath), ".env")
		if isDownloadSource(old) {
			m.pendingSource = old
			m.confirm = confirmState{
				kind:        confirmCleanup,
				title:       "¿Eliminar el binario de la carpeta de descargas?",
				affirmative: "Sí, eliminar",
				negative:    "No, mantener",
				yes:         true,
			}
			m.screen = scrConfirm
			return m, nil
		}
		return m.enterProviders(), nil

	case confirmCleanup:
		if yes {
			_ = os.Remove(m.pendingSource)
		}
		return m.enterProviders(), nil
	}
	return m, nil
}

func (m Model) afterConfirmAdd() (Model, tea.Cmd) {
	if m.binaryPath == "" {
		if path, err := findBinary(); err == nil {
			m.binaryPath = path
		} else {
			m.pathInput = textinput.New()
			m.pathInput.Placeholder = "C:\\ruta\\a\\somm.exe"
			m.pathInput.Focus()
			m.screen = scrBinaryPathInput
			return m, textinput.Blink
		}
	}
	return m.afterBinaryResolved()
}

func (m Model) afterBinaryResolved() (Model, tea.Cmd) {
	target, alreadyCorrect, targetExists := resolveInstallTarget(m.binaryPath)
	m.pendingTarget = target

	if alreadyCorrect {
		return m.enterProviders(), nil
	}

	if targetExists {
		m.confirm = confirmState{
			kind:        confirmReplaceBinary,
			title:       fmt.Sprintf("Ya existe un somm en %s. ¿Reemplazar?", filepath.Dir(target)),
			affirmative: "Sí, reemplazar",
			negative:    "No, mantener el actual",
			yes:         true,
		}
		m.screen = scrConfirm
		return m, nil
	}

	if err := installBinary(m.binaryPath, target); err != nil {
		m.err = err
		m.resultLines = []string{fmt.Sprintf("Error instalando binario: %v", err)}
		m.screen = scrConfigResult
		return m, nil
	}

	old := m.binaryPath
	m.binaryPath = target
	m.envPath = filepath.Join(filepath.Dir(m.binaryPath), ".env")

	if isDownloadSource(old) {
		m.pendingSource = old
		m.confirm = confirmState{
			kind:        confirmCleanup,
			title:       "¿Eliminar el binario de la carpeta de descargas?",
			affirmative: "Sí, eliminar",
			negative:    "No, mantener",
			yes:         true,
		}
		m.screen = scrConfirm
		return m, nil
	}

	return m.enterProviders(), nil
}

func (m Model) enterProviders() Model {
	m.providers = defaultProviders()
	m.provCursor = 0
	m.screen = scrProviders
	return m
}

func (m Model) updateBinaryPathInput(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.screen = scrMenu
		return m, nil
	case "enter":
		path := strings.TrimSpace(m.pathInput.Value())
		if path == "" {
			return m, nil
		}
		m.binaryPath = path
		return m.afterBinaryResolved()
	}
	var cmd tea.Cmd
	m.pathInput, cmd = m.pathInput.Update(msg)
	return m, cmd
}

func (m Model) updateProviders(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		m.provCursor = (m.provCursor + 1) % len(m.providers)
	case "k", "up":
		m.provCursor = (m.provCursor - 1 + len(m.providers)) % len(m.providers)
	case " ":
		if !m.providers[m.provCursor].required {
			m.providers[m.provCursor].selected = !m.providers[m.provCursor].selected
		}
	case "enter":
		var selected []string
		for _, p := range m.providers {
			if p.selected {
				selected = append(selected, p.id)
			}
		}
		if err := validateProviders(selected); err != nil {
			return m, nil
		}
		m.keys = map[string]string{}
		m.keyFields = buildKeyFields(selected)
		m.keyIdx = 0
		m.keyErr = ""
		if len(m.keyFields) > 0 {
			m.keyFields[0].input.Focus()
			m.screen = scrProviderKeyInput
			return m, textinput.Blink
		}
	case "esc":
		m.screen = scrMenu
	}
	return m, nil
}

func (m Model) updateProviderKeyInput(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if m.keyIdx == 0 {
			m.screen = scrProviders
			return m, nil
		}
		m.keyFields[m.keyIdx].input.Blur()
		m.keyIdx--
		m.keyFields[m.keyIdx].input.Focus()
		return m, textinput.Blink
	case "enter", "tab":
		cur := &m.keyFields[m.keyIdx]
		value := strings.TrimSpace(cur.input.Value())
		if cur.provider == "OpenCode" && value == "" {
			m.keyErr = "OpenCode API Key es requerida"
			return m, nil
		}
		m.keyErr = ""
		m.keys[keyNameForProvider(cur.provider)] = value
		cur.input.Blur()

		if m.keyIdx == len(m.keyFields)-1 {
			if m.skipTierScreen {
				// The tier is already persisted in .env and --force was not
				// passed: skip the screen, but re-seed the value so
				// saveEnvFile's whole-file rewrite doesn't drop it
				// (setup-wizard spec, Requirement: Tier Capture "not
				// re-asked").
				m.keys["SOMM_OC_TIER"] = m.presetTier
				m.screen = scrConfigProgress
				return m, tea.Batch(m.spinner.Tick, doSaveConfigCmd(m))
			}
			m.tiers = []string{"go", "zen"}
			m.tierCursor = tierCursorFor(m.presetTier, m.tiers)
			m.screen = scrTier
			return m, nil
		}
		m.keyIdx++
		m.keyFields[m.keyIdx].input.Focus()
		return m, textinput.Blink
	}
	var cmd tea.Cmd
	m.keyFields[m.keyIdx].input, cmd = m.keyFields[m.keyIdx].input.Update(msg)
	return m, cmd
}

// tierCursorFor returns the index of preset within tiers, or 0 (the first
// option) when preset is empty or not found — the cursor default when
// there is nothing to preselect.
func tierCursorFor(preset string, tiers []string) int {
	for i, t := range tiers {
		if t == preset {
			return i
		}
	}
	return 0
}

// updateTier handles the OpenCode tier (go|zen) capture screen: up/down
// move the cursor, enter persists the selection into m.keys and advances
// to scrConfigProgress, esc returns to the last key field without skipping
// the tier — D1 fixes the answer domain to go|zen, there is no "none"
// choice (setup-wizard spec, Requirement: Tier Capture).
func (m Model) updateTier(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		m.tierCursor = (m.tierCursor + 1) % len(m.tiers)
	case "k", "up":
		m.tierCursor = (m.tierCursor - 1 + len(m.tiers)) % len(m.tiers)
	case "enter":
		m.keys["SOMM_OC_TIER"] = m.tiers[m.tierCursor]
		m.screen = scrConfigProgress
		return m, tea.Batch(m.spinner.Tick, doSaveConfigCmd(m))
	case "esc":
		m.screen = scrProviderKeyInput
		m.keyIdx = len(m.keyFields) - 1
		m.keyFields[m.keyIdx].input.Focus()
		return m, textinput.Blink
	}
	return m, nil
}

type updateDoneMsg struct {
	err           error
	latestVersion string
}

func doUpdateCmd(version string, fn func(string) error) tea.Cmd {
	return func() tea.Msg {
		err := fn(version)
		return updateDoneMsg{err: err, latestVersion: version}
	}
}

type configDoneMsg struct {
	err   error
	steps []string
}

func doSaveConfigCmd(m Model) tea.Cmd {
	envPath := m.envPath
	keys := m.keys
	config := m.config
	configPath := m.configPath
	binaryPath := m.binaryPath
	saveEnvFileFn := m.saveEnvFileFn
	writeConfigFn := m.writeConfigFn
	createAliasFn := m.createAliasFn

	return func() tea.Msg {
		var steps []string

		if err := saveEnvFileFn(envPath, keys); err != nil {
			return configDoneMsg{err: fmt.Errorf("guardando .env: %w", err)}
		}
		steps = append(steps, fmt.Sprintf("%s .env guardado en %s", successStyle.Render("✓"), detailStyle.Render(envPath)))

		if err := updateMCPConfig(config, binaryPath); err != nil {
			return configDoneMsg{err: fmt.Errorf("preparando entrada MCP: %w", err), steps: steps}
		}
		if err := writeConfigFn(configPath, config); err != nil {
			return configDoneMsg{err: fmt.Errorf("actualizando opencode.json: %w", err), steps: steps}
		}
		steps = append(steps, successStyle.Render("✓")+" opencode.json actualizado")

		if err := createAliasFn(); err != nil {
			steps = append(steps, fmt.Sprintf("%s alias msetup no se pudo crear: %s", warnStyle.Render("⚠"), detailStyle.Render(err.Error())))
		} else {
			steps = append(steps, successStyle.Render("✓")+" alias msetup creado "+detailStyle.Render("(reiniciá PowerShell para usarlo)"))
		}

		return configDoneMsg{steps: steps}
	}
}

func (m Model) View() string {
	if m.quitting {
		return ""
	}
	switch m.screen {
	case scrMenu:
		return m.viewMenu()
	case scrStatus:
		return m.viewStatus()
	case scrConfirm:
		return m.viewConfirm()
	case scrUpdateProgress:
		return renderScreen("Actualizando somm", fmt.Sprintf("%s Descargando %s...", m.spinner.View(), m.latestVersion), "")
	case scrUpdateResult, scrConfigResult:
		return m.viewResult()
	case scrBinaryPathInput:
		return renderScreen("Ubicación del binario", fmt.Sprintf("¿Dónde está el binario somm?\n\n%s", m.pathInput.View()), helpConfirm)
	case scrProviders:
		return m.viewProviders()
	case scrProviderKeyInput:
		return m.viewKeyInput()
	case scrConfigProgress:
		return renderScreen("Configurando somm", fmt.Sprintf("%s Guardando configuración...", m.spinner.View()), "")
	case scrTier:
		return m.viewTier()
	}
	return ""
}

func (m Model) viewMenu() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("somm %s\n\n", m.currentVersion))
	for i, e := range m.menu {
		b.WriteString(menuItem(e.label, i == m.menuCursor))
		b.WriteString("\n")
	}
	return renderScreen("Somm Setup Wizard", b.String(), helpNavigate)
}

func (m Model) viewConfirm() string {
	yesLabel := m.confirm.affirmative
	noLabel := m.confirm.negative
	if m.confirm.yes {
		yesLabel = selectedItemStyle.Render(yesLabel)
	} else {
		noLabel = selectedItemStyle.Render(noLabel)
	}
	body := fmt.Sprintf("%s\n\n%s    %s", m.confirm.title, yesLabel, noLabel)
	return renderScreen("", body, helpConfirm)
}

func (m Model) viewResult() string {
	var b strings.Builder
	if m.err != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("✗ %v", m.err)))
		b.WriteString("\n\n")
	}
	for _, line := range m.resultLines {
		b.WriteString(line)
		b.WriteString("\n")
	}
	return renderScreen("Resultado", b.String(), helpContinue)
}

func (m Model) viewProviders() string {
	var b strings.Builder
	b.WriteString("¿Qué proveedores vas a usar?\n\n")
	for i, p := range m.providers {
		mark := "[ ]"
		if p.selected {
			mark = "[x]"
		}
		b.WriteString(menuItem(fmt.Sprintf("%s %s", mark, p.label), i == m.provCursor))
		b.WriteString("\n")
	}
	return renderScreen("Configurar somm", b.String(), "j/k: navigate • space: toggle • enter: continue • esc: back")
}

func (m Model) viewKeyInput() string {
	f := m.keyFields[m.keyIdx]
	body := fmt.Sprintf("%s\n\n%s", f.title, f.input.View())
	if m.keyErr != "" {
		body += "\n\n" + errorStyle.Render(m.keyErr)
	}
	return renderScreen("Configurar somm", body, helpContinue)
}

func (m Model) viewTier() string {
	var b strings.Builder
	b.WriteString("¿Qué tier de OpenCode tenés?\n\n")
	for i, t := range m.tiers {
		b.WriteString(menuItem(t, i == m.tierCursor))
		b.WriteString("\n")
	}
	return renderScreen("Configurar somm", b.String(), "j/k: navigate • enter: select • esc: back • q: quit")
}

func (m Model) viewStatus() string {
	var b strings.Builder
	b.WriteString(successStyle.Render("✓ somm ya está configurado"))
	b.WriteString(fmt.Sprintf("\n  Ubicación: %s\n\n", detailStyle.Render(m.binaryPath)))

	if _, err := os.Stat(m.envPath); err == nil {
		b.WriteString(successStyle.Render("✓ .env encontrado") + "\n")
	} else {
		b.WriteString(errorStyle.Render("✗ .env no encontrado") + "\n")
	}
	b.WriteString(successStyle.Render("✓ Habilitado en OpenCode") + "\n\n")

	b.WriteString(fmt.Sprintf("Tools disponibles: %d\n", len(sommTools)))
	for _, t := range sommTools {
		b.WriteString("  - " + t + "\n")
	}
	return renderScreen("Estado", b.String(), helpContinue)
}
