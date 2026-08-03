package profile

import (
	_ "embed"
	"fmt"
)

//go:embed presets/gentle-ai.yaml
var gentleAIPreset []byte

// Preset loads and returns the embedded gentle-ai default profile. It runs
// through the same validating loader used for external files, so any drift
// in the embedded YAML fails at build/test time instead of silently.
func Preset() (*Profile, error) {
	p, err := Load(gentleAIPreset)
	if err != nil {
		return nil, fmt.Errorf("loading embedded gentle-ai preset: %w", err)
	}
	return p, nil
}
