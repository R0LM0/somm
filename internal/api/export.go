package api

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/R0LM0/somm/v2/internal/profile"
)

// ConfigChange describes a single model change for an agent.
type ConfigChange struct {
	Agent    string `json:"agent"`
	OldModel string `json:"oldModel"`
	NewModel string `json:"newModel"`
	Applied  bool   `json:"applied"`
}

// ExportResult holds the output of ExportConfig.
type ExportResult struct {
	Content string         `json:"content"`
	Path    string         `json:"path"`
	Changes []ConfigChange `json:"changes"`
}

// ExportConfig reads the existing opencode.json, gets model recommendations,
// and returns an updated config with ONLY the "model" field changed per agent.
// It never writes to disk — the caller decides.
func ExportConfig(ctx context.Context, client *Client, prof *profile.Profile, opencodePath string, roles []string) (ExportResult, error) {
	if opencodePath == "" {
		opencodePath = detectConfigPath()
	}

	existing, err := readExistingConfig(opencodePath)
	if err != nil {
		return ExportResult{}, fmt.Errorf("reading config: %w", err)
	}

	_, recs, err := RecommendConfig(ctx, client, prof, roles)
	if err != nil {
		return ExportResult{}, fmt.Errorf("generating recommendations: %w", err)
	}

	changes := applyModelUpdates(existing, recs)

	updatedJSON, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return ExportResult{}, fmt.Errorf("marshaling config: %w", err)
	}

	return ExportResult{
		Content: string(updatedJSON),
		Path:    opencodePath,
		Changes: changes,
	}, nil
}

// detectConfigPath returns the default opencode.json path based on OS.
func detectConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "opencode", "opencode.json")
}

// readExistingConfig reads and parses the opencode.json file. Returns nil map if file doesn't exist.
func readExistingConfig(path string) (map[string]any, error) {
	if path == "" {
		return make(map[string]any), nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]any), nil
		}
		return nil, err
	}

	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return config, nil
}

// applyModelUpdates finds each recommended agent in the config and updates
// ONLY the "model" field. Returns the list of changes attempted.
func applyModelUpdates(config map[string]any, recs []Recommendation) []ConfigChange {
	changes := make([]ConfigChange, 0, len(recs))

	agents, ok := config["agents"]
	if !ok || agents == nil {
		// No agents block — create nothing, just report what would change.
		for _, r := range recs {
			if r.Model == "" {
				continue
			}
			changes = append(changes, ConfigChange{
				Agent:    r.Agent,
				NewModel: r.OCID,
				Applied:  false,
			})
		}
		return changes
	}

	agentsMap, ok := agents.(map[string]any)
	if !ok {
		for _, r := range recs {
			if r.Model == "" {
				continue
			}
			changes = append(changes, ConfigChange{
				Agent:    r.Agent,
				NewModel: r.OCID,
				Applied:  false,
			})
		}
		return changes
	}

	for _, r := range recs {
		if r.Model == "" {
			continue
		}

		change := ConfigChange{
			Agent:    r.Agent,
			NewModel: r.OCID,
		}

		agentCfg, ok := agentsMap[r.Agent]
		if !ok || agentCfg == nil {
			// Agent not in config — report but don't create.
			change.Applied = false
			changes = append(changes, change)
			continue
		}

		agentMap, ok := agentCfg.(map[string]any)
		if !ok {
			change.Applied = false
			changes = append(changes, change)
			continue
		}

		if old, ok := agentMap["model"].(string); ok {
			change.OldModel = old
		}

		agentMap["model"] = r.OCID
		change.Applied = true
		changes = append(changes, change)
	}

	return changes
}

// FormatExportResult builds the user-facing text output for export_config.
func FormatExportResult(result ExportResult) string {
	var sb strings.Builder

	sb.WriteString("Configuración recomendada para opencode.json:\n\n")

	if len(result.Changes) == 0 {
		sb.WriteString("No hay cambios disponibles.\n")
		return sb.String()
	}

	sb.WriteString("Cambios a aplicar:\n")
	for _, c := range result.Changes {
		old := c.OldModel
		if old == "" {
			old = "(no configurado)"
		}
		applied := ""
		if !c.Applied {
			applied = " [agente no encontrado en config]"
		}
		sb.WriteString(fmt.Sprintf("  - %s: %s → %s%s\n", c.Agent, old, c.NewModel, applied))
	}

	sb.WriteString("\n⚠️  IMPORTANTE: Esto solo actualiza el campo \"model\" de cada agente.\n")
	sb.WriteString("Tu config de Gentle AI (prompts, tools, permissions) NO se modifica.\n\n")

	sb.WriteString(fmt.Sprintf("¿Dónde está tu opencode.json?\n"))
	sb.WriteString(fmt.Sprintf("Ruta detectada: %s\n\n", result.Path))

	sb.WriteString("Para aplicar, copia el siguiente JSON y pégalo en tu config:\n")
	sb.WriteString("```json\n")
	sb.WriteString(result.Content)
	sb.WriteString("\n```\n")

	return sb.String()
}
