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

	"github.com/charmbracelet/huh"
)

type openCodeConfig struct {
	MCP map[string]mcpEntry `json:"mcp"`
}

type mcpEntry struct {
	Command []string `json:"command"`
	Enabled bool     `json:"enabled"`
	Type    string   `json:"type"`
}

func runSetup() {
	fmt.Println("🔧 Somm Setup Wizard")
	fmt.Println("==============================")
	fmt.Println()

	// Parse --force flag before other setup logic.
	force := false
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	fs.BoolVar(&force, "force", false, "Force reconfiguration")
	// Only parse if there are args after "setup" (e.g., somm setup --force)
	// When called from maybeRunSetup, os.Args may not have the "setup" arg
	if len(os.Args) >= 3 {
		_ = fs.Parse(os.Args[2:])
	}

	// Check for updates (skip if fails or same version)
	latestVersion, err := checkForUpdate()
	// Strip 'v' prefix for comparison (e.g., "v2.1.6" -> "2.1.6")
	latestClean := strings.TrimPrefix(latestVersion, "v")
	versionClean := strings.TrimPrefix(version, "v")
	if err == nil && latestClean != versionClean && latestClean != "" {
		fmt.Printf("📦 Nueva versión disponible: %s (actual: %s)\n\n", latestVersion, version)

		var update bool
		huh.NewConfirm().
			Title("¿Querés actualizar a la última versión?").
			Affirmative("Sí, actualizar").
			Negative("No, mantener").
			Value(&update).Run()

		if update {
			fmt.Println("\n🔄 Descargando actualización...")
			if err := updateBinary(latestVersion); err != nil {
				fmt.Printf("⚠️  No se pudo actualizar: %v\n", err)
				fmt.Println("Podés descargar manualmente desde: https://github.com/R0LM0/somm/releases")
			} else {
				fmt.Println("✅ Actualizado a", latestVersion)
				fmt.Println("🔄 Continuando con el setup...")
			}
		}
		fmt.Println()
	}

	// Step 1: Find OpenCode config
	configPath, err := findOpenCodeConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ OpenCode no encontrado: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ opencode.json encontrado en: %s\n\n", configPath)

	// Step 2: Read existing config
	config, err := readConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error leyendo config: %v\n", err)
		os.Exit(1)
	}

	// Step 3: Resolve binary path and check configuration state.
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

	if alreadyConfigured && !force {
		runStatus(binaryPath, envPath)
		return
	}

	// Not configured or force: guide through setup.
	runConfigurationFlow(binaryPath, envPath, config, configPath)
}

// resolveBinaryAndEnv returns the resolved binary path and the .env path next
// to it. It prefers the path recorded in opencode.json when available.
func resolveBinaryAndEnv(config *openCodeConfig) (binaryPath, envPath string, err error) {
	if entry, exists := config.MCP["somm"]; exists && len(entry.Command) > 0 && entry.Command[0] != "" {
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

	entry, exists := config.MCP["somm"]
	if !exists || len(entry.Command) == 0 || entry.Command[0] == "" {
		return false, ""
	}

	return true, entry.Command[0]
}

func runStatus(binaryPath, envPath string) {
	fmt.Printf("✅ somm ya está configurado\n")
	fmt.Printf("   Ubicación: %s\n\n", binaryPath)

	fmt.Println("📋 Validando configuración...")
	fmt.Println()

	if _, err := os.Stat(envPath); err == nil {
		fmt.Println("✅ .env encontrado")
	} else {
		fmt.Println("❌ .env no encontrado")
	}

	fmt.Println("✅ Habilitado en OpenCode")
	fmt.Println("\n✅ Configuración válida!")
	fmt.Println("\nTools disponibles: 8")
	fmt.Println("   - list_available_models")
	fmt.Println("   - get_agent_criteria")
	fmt.Println("   - get_model_benchmarks")
	fmt.Println("   - recommend_config")
	fmt.Println("   - estimate_cost")
	fmt.Println("   - compare_models")
	fmt.Println("   - validate_config")
	fmt.Println("   - export_config")

	fmt.Println("\nPara reconfigurar: somm setup --force")
}

func runConfigurationFlow(binaryPath, envPath string, config *openCodeConfig, configPath string) {
	fmt.Println("❌ somm no está configurado")
	fmt.Println()

	var add bool
	huh.NewConfirm().
		Title("¿Querés agregarlo a OpenCode?").
		Affirmative("Sí, agregar").
		Negative("No, cancelar").
		Value(&add).Run()

	if !add {
		fmt.Println("\nCancelado.")
		return
	}

	fmt.Println()

	// Resolve binary path if not already set.
	if binaryPath == "" {
		if path, err := findBinary(); err == nil {
			binaryPath = path
		} else {
			huh.NewInput().
				Title("¿Dónde está el binario somm?").
				Value(&binaryPath).Run()
		}
	}

	// Ensure binary is in the correct location ($GOPATH/bin)
	binaryPath, err := ensureBinaryInCorrectLocation(binaryPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error moviendo binario: %v\n", err)
		fmt.Println("Podés moverlo manualmente a $GOPATH/bin/somm.exe")
	}

	envPath = filepath.Join(filepath.Dir(binaryPath), ".env")

	selected, err := providersSelected()
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error seleccionando proveedores: %v\n", err)
		os.Exit(1)
	}

	keys := map[string]string{}
	for _, provider := range selected {
		var title string
		switch provider {
		case "OpenCode":
			title = "OpenCode API Key (requerido)"
		case "OpenRouter":
			title = "OpenRouter API Key (opcional)"
		case "Kimi":
			title = "Kimi API Key (opcional)"
		}

		var value string
		huh.NewInput().
			Title(title).
			Value(&value).Run()
		value = strings.TrimSpace(value)

		if provider == "OpenCode" && value == "" {
			fmt.Fprintln(os.Stderr, "❌ OpenCode API Key es requerida")
			os.Exit(1)
		}
		keys[keyNameForProvider(provider)] = value
	}

	fmt.Println()
	if err := saveEnvFile(envPath, keys); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error guardando .env: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ .env guardado en: %s\n", envPath)

	updateMCPConfig(config, binaryPath)
	if err := writeConfig(configPath, config); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error actualizando config: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ opencode.json actualizado\n\n")

	fmt.Println("✅ Tools disponibles: 8")
	fmt.Println("   - list_available_models")
	fmt.Println("   - get_agent_criteria")
	fmt.Println("   - get_model_benchmarks")
	fmt.Println("   - recommend_config")
	fmt.Println("   - estimate_cost")
	fmt.Println("   - compare_models")
	fmt.Println("   - validate_config")
	fmt.Println("   - export_config")

	fmt.Println("\n==============================")
	fmt.Println("✅ Configuración completada!")
	fmt.Println("==============================")

	if err := createPowerShellAlias(); err != nil {
		fmt.Printf("⚠️  No se pudo crear alias automático: %v\n", err)
		fmt.Println("Creá manualmente: function msetup { somm setup }")
	} else {
		fmt.Println("\n✅ Alias creado: msetup")
		fmt.Println("   Reiniciá PowerShell para usarlo")
	}

	fmt.Println("\nPróximos pasos:")
	fmt.Println("1. Reiniciá OpenCode")
	fmt.Println("2. Abrí una nueva sesión")
	fmt.Println("3. Probá: \"¿Qué modelos tengo disponibles?\"")
	fmt.Println("\nPara reconfigurar: somm setup")
}

func providersSelected() ([]string, error) {
	var selected []string
	options := []huh.Option[string]{
		huh.NewOption("OpenCode Go/Zen (requerido)", "OpenCode").Selected(true),
		huh.NewOption("OpenRouter (opcional)", "OpenRouter"),
		huh.NewOption("Kimi (opcional)", "Kimi"),
	}

	err := huh.NewMultiSelect[string]().
		Title("¿Qué proveedores vas a usar?").
		Options(options...).
		Value(&selected).
		Validate(validateProviders).
		Run()
	if err != nil {
		return nil, err
	}
	return selected, nil
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
	case "Kimi":
		return "KIMI_API_KEY"
	default:
		return ""
	}
}

func saveEnvFile(envPath string, keys map[string]string) error {
	var envContent strings.Builder
	order := []string{"OPENCODE_API_KEY", "OPENROUTER_API_KEY", "KIMI_API_KEY"}
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

func updateMCPConfig(config *openCodeConfig, binaryPath string) {
	config.MCP["somm"] = mcpEntry{
		Command: []string{binaryPath},
		Enabled: true,
		Type:    "local",
	}
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

	var config openCodeConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	if config.MCP == nil {
		config.MCP = make(map[string]mcpEntry)
	}

	return &config, nil
}

func writeConfig(path string, config *openCodeConfig) error {
	data, err := json.MarshalIndent(config, "", "  ")
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

// ensureBinaryInCorrectLocation checks if the binary is in $GOPATH/bin and
// copies it there if not. Returns the final path to use.
func ensureBinaryInCorrectLocation(currentPath string) (string, error) {
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
	targetPath := filepath.Join(targetDir, binName)

	// If already in correct location, return as-is
	if currentPath == targetPath {
		return targetPath, nil
	}

	// Check if target already exists
	if _, err := os.Stat(targetPath); err == nil {
		// Target exists, ask user if they want to replace
		var replace bool
		huh.NewConfirm().
			Title(fmt.Sprintf("Ya existe un somm en %s. ¿Reemplazar?", targetDir)).
			Affirmative("Sí, reemplazar").
			Negative("No, mantener el actual").
			Value(&replace).Run()

		if !replace {
			return currentPath, nil
		}
	}

	// Create target directory if needed
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return currentPath, fmt.Errorf("creando directorio: %w", err)
	}

	// Read source binary
	sourceData, err := os.ReadFile(currentPath)
	if err != nil {
		return currentPath, fmt.Errorf("leyendo binario: %w", err)
	}

	// Write to target
	if err := os.WriteFile(targetPath, sourceData, 0755); err != nil {
		return currentPath, fmt.Errorf("escribiendo binario: %w", err)
	}

	fmt.Printf("✅ Binario instalado en: %s\n", targetPath)

	// If source was in a temp/downloads folder, offer to clean up
	if strings.Contains(currentPath, "Downloads") || strings.Contains(currentPath, "Temp") {
		var cleanup bool
		huh.NewConfirm().
			Title("¿Eliminar el binario de la carpeta de descargas?").
			Affirmative("Sí, eliminar").
			Negative("No, mantener").
			Value(&cleanup).Run()

		if cleanup {
			os.Remove(currentPath)
			fmt.Println("✅ Binario original eliminado")
		}
	}

	return targetPath, nil
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

	// Replace current binary
	if err := os.Rename(extractedBinary, currentPath); err != nil {
		return fmt.Errorf("error reemplazando binario: %w", err)
	}

	return nil
}
