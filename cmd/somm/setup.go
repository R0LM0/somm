package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// openCodeConfig holds opencode.json as raw JSON, not a typed struct.
// opencode.json commonly carries agent/model assignments, permissions, and
// other MCP servers that somm has no business knowing the shape of — the
// only thing somm ever needs to touch is the "somm" entry inside "mcp".
// Round-tripping through a struct that only declares MCP would silently
// drop every other key on write (see raw/MCP below).
type openCodeConfig struct {
	raw map[string]json.RawMessage // the full document, byte-for-byte except "mcp"
	MCP map[string]json.RawMessage // the "mcp" object, byte-for-byte except the "somm" key
}

type mcpEntry struct {
	Command []string `json:"command"`
	Enabled bool     `json:"enabled"`
	Type    string   `json:"type"`
}

// sommEntry decodes the "somm" MCP entry, if present and well-formed.
func (c *openCodeConfig) sommEntry() (mcpEntry, bool) {
	raw, ok := c.MCP["somm"]
	if !ok {
		return mcpEntry{}, false
	}
	var entry mcpEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		return mcpEntry{}, false
	}
	return entry, true
}

// runSetup runs the non-interactive preflight (parse --force, check for
// updates, locate/read opencode.json, resolve the binary and .env paths,
// check whether somm is already configured) and hands the result to the
// Bubble Tea wizard, which owns every interactive step from there.
func runSetup() {
	force := false
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	fs.BoolVar(&force, "force", false, "Force reconfiguration")
	// Only parse if there are args after "setup" (e.g., somm setup --force)
	// When called from maybeRunSetup, os.Args may not have the "setup" arg
	if len(os.Args) >= 3 {
		_ = fs.Parse(os.Args[2:])
	}

	fmt.Println("🔧 Somm Setup Wizard")
	fmt.Println("==============================")
	fmt.Println()

	// The wizard is a Bubble Tea program and needs a real controlling
	// terminal (it opens /dev/tty directly for raw input on Unix). Bail out
	// with a clear message instead of letting tea.Program crash when stdin
	// isn't a terminal (piped/non-interactive contexts, e.g. CI).
	if !isTerminal() {
		fmt.Println("No hay una terminal interactiva disponible.")
		fmt.Println("Corré \"somm setup\" desde una terminal real para usar el wizard.")
		return
	}

	latestVersion, err := checkForUpdate()
	latestClean := strings.TrimPrefix(latestVersion, "v")
	versionClean := strings.TrimPrefix(version, "v")
	updateAvailable := err == nil && latestClean != "" && latestClean != versionClean

	configPath, err := findOpenCodeConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ OpenCode no encontrado: %v\n", err)
		os.Exit(1)
	}

	config, err := readConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error leyendo config: %v\n", err)
		os.Exit(1)
	}

	binaryPath, envPath, err := resolveBinaryAndEnv(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error resolviendo binario: %v\n", err)
		os.Exit(1)
	}

	alreadyConfigured, configuredBinaryPath := isAlreadyConfigured(envPath, config)
	if configuredBinaryPath != "" {
		binaryPath = configuredBinaryPath
		envPath = filepath.Join(filepath.Dir(binaryPath), ".env")
	}

	// The tier-presence check is independent of alreadyConfigured (keys):
	// an existing install upgrading onto this feature has keys but no
	// persisted tier yet, so it must still see the tier screen once.
	presetTier := existingTier(envPath)
	skipTierScreen := presetTier != "" && !force

	m := newModel(modelInit{
		configPath:        configPath,
		config:            config,
		binaryPath:        binaryPath,
		envPath:           envPath,
		alreadyConfigured: alreadyConfigured && !force,
		updateAvailable:   updateAvailable,
		latestVersion:     latestVersion,
		currentVersion:    version,
		presetTier:        presetTier,
		skipTierScreen:    skipTierScreen,
	})

	if _, err := tea.NewProgram(m).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error en la TUI: %v\n", err)
		os.Exit(1)
	}
}

// resolveBinaryAndEnv returns the resolved binary path and the .env path next
// to it. It prefers the path recorded in opencode.json when available.
func resolveBinaryAndEnv(config *openCodeConfig) (binaryPath, envPath string, err error) {
	if entry, ok := config.sommEntry(); ok && len(entry.Command) > 0 && entry.Command[0] != "" {
		binaryPath = entry.Command[0]
	} else if binaryPath, err = findBinary(); err != nil {
		binaryPath, _ = os.Executable()
	}
	if binaryPath == "" {
		return "", "", errors.New("no se pudo resolver la ubicación del binario")
	}
	return binaryPath, filepath.Join(filepath.Dir(binaryPath), ".env"), nil
}

// isAlreadyConfigured reports whether .env contains OPENCODE_API_KEY and
// opencode.json contains a valid somm MCP entry. When configured, it returns
// the binary path from the MCP entry.
func isAlreadyConfigured(envPath string, config *openCodeConfig) (bool, string) {
	data, err := os.ReadFile(envPath)
	if err != nil {
		return false, ""
	}

	ocKey := ""
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "OPENCODE_API_KEY=") {
			ocKey = strings.TrimPrefix(line, "OPENCODE_API_KEY=")
			break
		}
	}
	if strings.TrimSpace(ocKey) == "" {
		return false, ""
	}

	entry, ok := config.sommEntry()
	if !ok || len(entry.Command) == 0 || entry.Command[0] == "" {
		return false, ""
	}

	return true, entry.Command[0]
}

// existingTier reads the persisted SOMM_OC_TIER value from .env, if any. It
// returns "" when the file is missing or the key is absent. This check is
// deliberately independent of isAlreadyConfigured's key-presence check: an
// existing install upgrading onto this feature can have OPENCODE_API_KEY
// persisted with no SOMM_OC_TIER yet, so reusing alreadyConfigured to skip
// the tier screen would mean that upgrading user never gets asked
// (setup-wizard spec, Requirement: Tier Capture).
func existingTier(envPath string) string {
	data, err := os.ReadFile(envPath)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "SOMM_OC_TIER=") {
			return strings.TrimPrefix(line, "SOMM_OC_TIER=")
		}
	}
	return ""
}

func validateProviders(v []string) error {
	if !slices.Contains(v, "OpenCode") {
		return errors.New("OpenCode es requerido")
	}
	return nil
}

func keyNameForProvider(provider string) string {
	switch provider {
	case "OpenCode":
		return "OPENCODE_API_KEY"
	case "OpenRouter":
		return "OPENROUTER_API_KEY"
	default:
		return ""
	}
}

func saveEnvFile(envPath string, keys map[string]string) error {
	var envContent strings.Builder
	order := []string{"OPENCODE_API_KEY", "OPENROUTER_API_KEY", "SOMM_OC_TIER"}
	for _, key := range order {
		if value, ok := keys[key]; ok && value != "" {
			envContent.WriteString(fmt.Sprintf("%s=%s\n", key, value))
		}
	}
	if envContent.Len() == 0 {
		return nil
	}
	return os.WriteFile(envPath, []byte(envContent.String()), 0600)
}

// updateMCPConfig sets the "somm" entry inside config.MCP, leaving every
// other MCP entry and every other top-level key untouched.
func updateMCPConfig(config *openCodeConfig, binaryPath string) error {
	entry := mcpEntry{
		Command: []string{binaryPath},
		Enabled: true,
		Type:    "local",
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("encoding somm MCP entry: %w", err)
	}
	config.MCP["somm"] = raw
	return nil
}

func findOpenCodeConfig() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	path := filepath.Join(home, ".config", "opencode", "opencode.json")
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}

	return "", fmt.Errorf("opencode.json no encontrado en %s", filepath.Dir(path))
}

func readConfig(path string) (*openCodeConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if raw == nil {
		raw = map[string]json.RawMessage{}
	}

	mcp := map[string]json.RawMessage{}
	if mcpRaw, ok := raw["mcp"]; ok {
		if err := json.Unmarshal(mcpRaw, &mcp); err != nil {
			return nil, fmt.Errorf("parsing mcp entry: %w", err)
		}
	}
	if mcp == nil {
		mcp = map[string]json.RawMessage{}
	}

	return &openCodeConfig{raw: raw, MCP: mcp}, nil
}

// writeConfig writes config.raw back to disk after folding config.MCP into
// its "mcp" key — every other key, and every mcp entry besides "somm", comes
// back out byte-for-byte as it was read.
func writeConfig(path string, config *openCodeConfig) error {
	mcpData, err := json.MarshalIndent(config.MCP, "", "  ")
	if err != nil {
		return err
	}
	if config.raw == nil {
		config.raw = map[string]json.RawMessage{}
	}
	config.raw["mcp"] = mcpData

	data, err := json.MarshalIndent(config.raw, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func findBinary() (string, error) {
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		home, _ := os.UserHomeDir()
		gopath = filepath.Join(home, "go")
	}

	binName := "somm"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}

	path := filepath.Join(gopath, "bin", binName)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}

	return "", fmt.Errorf("binario no encontrado en %s", filepath.Join(gopath, "bin"))
}

// resolveInstallTarget reports where the somm binary should live
// ($GOPATH/bin), whether currentPath already points there, and whether a
// binary already exists at that target. It performs no writes.
func resolveInstallTarget(currentPath string) (targetPath string, alreadyCorrect, targetExists bool) {
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		home, _ := os.UserHomeDir()
		gopath = filepath.Join(home, "go")
	}

	binName := "somm"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}

	targetDir := filepath.Join(gopath, "bin")
	targetPath = filepath.Join(targetDir, binName)

	if currentPath == targetPath {
		return targetPath, true, false
	}

	_, err := os.Stat(targetPath)
	return targetPath, false, err == nil
}

// installBinary copies currentPath to targetPath, creating the target
// directory if needed.
func installBinary(currentPath, targetPath string) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return fmt.Errorf("creando directorio: %w", err)
	}

	sourceData, err := os.ReadFile(currentPath)
	if err != nil {
		return fmt.Errorf("leyendo binario: %w", err)
	}

	if err := os.WriteFile(targetPath, sourceData, 0755); err != nil {
		return fmt.Errorf("escribiendo binario: %w", err)
	}

	return nil
}

// isDownloadSource reports whether path looks like a Downloads/Temp folder,
// worth offering to clean up once the binary has been installed elsewhere.
func isDownloadSource(path string) bool {
	return strings.Contains(path, "Downloads") || strings.Contains(path, "Temp")
}

func createPowerShellAlias() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	// PowerShell profile path
	profilePath := filepath.Join(home, "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")

	// Check if profile exists, create if not
	if _, err := os.Stat(profilePath); os.IsNotExist(err) {
		profileDir := filepath.Dir(profilePath)
		if err := os.MkdirAll(profileDir, 0755); err != nil {
			return err
		}
		if err := os.WriteFile(profilePath, []byte("# PowerShell Profile\n"), 0644); err != nil {
			return err
		}
	}

	// Read existing profile
	profile, err := os.ReadFile(profilePath)
	if err != nil {
		return err
	}

	// Check if alias already exists
	alias := "function msetup { somm setup }"
	if strings.Contains(string(profile), "msetup") {
		return nil // Already exists
	}

	// Append alias
	profile = append(profile, []byte("\n# Somm alias\n"+alias+"\n")...)
	return os.WriteFile(profilePath, profile, 0644)
}

func checkForUpdate() (string, error) {
	resp, err := http.Get("https://api.github.com/repos/R0LM0/somm/releases/latest")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}

	return release.TagName, nil
}

func updateBinary(version string) error {
	// Determine OS and architecture
	osName := runtime.GOOS
	arch := runtime.GOARCH

	// Build download URL
	extension := ".tar.gz"
	if osName == "windows" {
		extension = ".zip"
	}

	// Strip leading 'v' from version for filename (e.g., "v2.1.4" -> "2.1.4")
	fileVersion := strings.TrimPrefix(version, "v")

	filename := fmt.Sprintf("somm_%s_%s_%s%s", fileVersion, osName, arch, extension)
	url := fmt.Sprintf("https://github.com/R0LM0/somm/releases/download/%s/%s", version, filename)

	// Download
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("error descargando: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("error descargando: status %d", resp.StatusCode)
	}

	// Get current binary path
	currentPath, err := os.Executable()
	if err != nil {
		return err
	}

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "somm-update-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	// Save archive
	archivePath := filepath.Join(tmpDir, filename)
	archiveFile, err := os.Create(archivePath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(archiveFile, resp.Body); err != nil {
		archiveFile.Close()
		return err
	}
	archiveFile.Close()

	// Extract archive
	if osName == "windows" {
		// Use PowerShell to extract zip
		cmd := exec.Command("powershell", "-Command", fmt.Sprintf("Expand-Archive -Path '%s' -DestinationPath '%s' -Force", archivePath, tmpDir))
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("error extrayendo zip: %w", err)
		}
	} else {
		// Use tar for unix
		cmd := exec.Command("tar", "-xzf", archivePath, "-C", tmpDir)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("error extrayendo tar.gz: %w", err)
		}
	}

	// Find the binary in extracted files
	binaryName := "somm"
	if osName == "windows" {
		binaryName += ".exe"
	}

	var extractedBinary string
	err = filepath.Walk(tmpDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Name() == binaryName {
			extractedBinary = path
		}
		return nil
	})
	if err != nil || extractedBinary == "" {
		return fmt.Errorf("binario no encontrado en el archivo")
	}

	// Make executable (Unix)
	if osName != "windows" {
		if err := os.Chmod(extractedBinary, 0755); err != nil {
			return err
		}
	}

	// Replace current binary. On Windows a running executable cannot be
	// overwritten in place, but it can be renamed, so move it aside first
	// and only then put the new binary in its original path.
	oldPath := currentPath + ".old"
	_ = os.Remove(oldPath) // clean up a leftover from a previous update, if any
	if err := os.Rename(currentPath, oldPath); err != nil {
		return fmt.Errorf("error moviendo binario actual: %w", err)
	}
	if err := os.Rename(extractedBinary, currentPath); err != nil {
		_ = os.Rename(oldPath, currentPath) // restore so the tool keeps working
		return fmt.Errorf("error reemplazando binario: %w", err)
	}
	_ = os.Remove(oldPath) // best effort; may still be locked while running

	return nil
}
