package main

import (
	"encoding/json"
	"fmt"
	"os"
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
	fmt.Println("🔧 Model Advisor Setup Wizard")
	fmt.Println("==============================")
	fmt.Println()

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
	if entry, exists := config.MCP["model-advisor"]; exists {
		if len(entry.Command) > 0 {
			binaryPath = entry.Command[0]
			alreadyConfigured = true
		}
	}

	if alreadyConfigured {
		fmt.Printf("✅ model-advisor ya está configurado\n")
		fmt.Printf("   Ubicación: %s\n\n", binaryPath)

		var update bool
		huh.NewConfirm().
			Title("¿Querés actualizar la configuración?").
			Affirmative("Sí, actualizar").
			Negative("No, mantener").
			Value(&update).Run()

		if !update {
			fmt.Println("\nManteniendo configuración actual.")
			return
		}
	} else {
		fmt.Println("❌ model-advisor no está configurado")
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
	}

	fmt.Println()

	// Step 4: Get binary path if not set
	if !alreadyConfigured {
		if path, err := findBinary(); err == nil {
			binaryPath = path
		} else {
			huh.NewInput().
				Title("¿Dónde está el binario model-advisor?").
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
	config.MCP["model-advisor"] = mcpEntry{
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
	fmt.Println("\nPróximos pasos:")
	fmt.Println("1. Reiniciá OpenCode")
	fmt.Println("2. Abrí una nueva sesión")
	fmt.Println("3. Probá: \"¿Qué modelos tengo disponibles?\"")
	fmt.Println("\nPara reconfigurar: model-advisor setup")
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
