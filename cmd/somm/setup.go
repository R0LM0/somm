package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

	// Check for updates
 latestVersion, err := checkForUpdate()
	if err == nil && latestVersion != version {
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
				fmt.Printf("❌ Error actualizando: %v\n", err)
			} else {
				fmt.Println("✅ Actualizado a", latestVersion)
				fmt.Println("🔄 Reiniciá el setup con: somm setup")
				return
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

	// Step 3: Check if already configured
	binaryPath, _ := os.Executable()
	alreadyConfigured := false
	if entry, exists := config.MCP["somm"]; exists {
		if len(entry.Command) > 0 {
			binaryPath = entry.Command[0]
			alreadyConfigured = true
		}
	}

	if alreadyConfigured {
		fmt.Printf("✅ somm ya está configurado\n")
		fmt.Printf("   Ubicación: %s\n\n", binaryPath)

		// Validate existing config
		fmt.Println("📋 Validando configuración...")
		fmt.Println()

		// Check .env
		envPath := filepath.Join(filepath.Dir(binaryPath), ".env")
		if _, err := os.Stat(envPath); err == nil {
			fmt.Println("✅ .env encontrado")
		} else {
			fmt.Println("❌ .env no encontrado")
		}

		// Check OpenCode config
		if entry, exists := config.MCP["somm"]; exists {
			if entry.Enabled {
				fmt.Println("✅ Habilitado en OpenCode")
			} else {
				fmt.Println("⚠️  Deshabilitado en OpenCode")
			}
		}

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
		return
	}

	// Not configured - guide through setup
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

	// Step 4: Get binary path if not set
	if !alreadyConfigured {
		if path, err := findBinary(); err == nil {
			binaryPath = path
		} else {
			huh.NewInput().
				Title("¿Dónde está el binario somm?").
				Value(&binaryPath).Run()
		}
	}

	// Step 5: Configure API keys
	fmt.Println("🔑 Configurar API keys (Enter para omitir):")
	fmt.Println()

	var ocKey, orKey string

	huh.NewInput().
		Title("OpenCode API Key (requerido)").
		Value(&ocKey).Run()

	huh.NewInput().
		Title("OpenRouter API Key (opcional)").
		Value(&orKey).Run()

	// Step 6: Save .env
	fmt.Println()
	envPath := filepath.Join(filepath.Dir(binaryPath), ".env")

	if ocKey != "" || orKey != "" {
		var envContent strings.Builder
		if ocKey != "" {
			envContent.WriteString(fmt.Sprintf("OPENCODE_API_KEY=%s\n", ocKey))
		}
		if orKey != "" {
			envContent.WriteString(fmt.Sprintf("OPENROUTER_API_KEY=%s\n", orKey))
		}

		if err := os.WriteFile(envPath, []byte(envContent.String()), 0600); err != nil {
			fmt.Printf("⚠️  Error guardando .env: %v\n", err)
		} else {
			fmt.Printf("✅ .env guardado en: %s\n", envPath)
		}
	}

	// Step 7: Update OpenCode config
	config.MCP["somm"] = mcpEntry{
		Command: []string{binaryPath},
		Enabled: true,
		Type:    "local",
	}

	if err := writeConfig(configPath, config); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error actualizando config: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ opencode.json actualizado\n\n")

	// Step 8: Show tools
	fmt.Println("✅ Tools disponibles: 8")
	fmt.Println("   - list_available_models")
	fmt.Println("   - get_agent_criteria")
	fmt.Println("   - get_model_benchmarks")
	fmt.Println("   - recommend_config")
	fmt.Println("   - estimate_cost")
	fmt.Println("   - compare_models")
	fmt.Println("   - validate_config")
	fmt.Println("   - export_config")

	// Summary
	fmt.Println("\n==============================")
	fmt.Println("✅ Configuración completada!")
	fmt.Println("==============================")

	// Create PowerShell alias
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

	binName := "server"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}

	path := filepath.Join(gopath, "bin", binName)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}

	return "", fmt.Errorf("binario no encontrado en %s", filepath.Join(gopath, "bin"))
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

	filename := fmt.Sprintf("somm_%s_%s_%s%s", version, osName, arch, extension)
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
	binaryName := "server"
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
