package profile

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	// envVar is the environment variable holding a profile file path.
	envVar = "SOMM_PROFILE"
	// localProfileFile is the conventional profile filename looked up in
	// the current working directory and under the XDG config directory.
	localProfileFile = "somm.yaml"
)

// Resolve determines and loads the active Profile using this precedence
// order: the --profile flag path, the SOMM_PROFILE environment variable,
// ./somm.yaml in the current working directory, an XDG config path
// ($XDG_CONFIG_HOME/somm/somm.yaml or ~/.config/somm/somm.yaml), and
// finally the embedded gentle-ai default preset.
//
// "No source present" falls through to the next source in the order and is
// never an error. A source that IS selected but fails to parse or validate
// returns a fatal error with no silent fallback to the embedded default.
func Resolve(flagPath string) (*Profile, error) {
	if flagPath != "" {
		return loadFile(flagPath)
	}

	if envPath := os.Getenv(envVar); envPath != "" {
		return loadFile(envPath)
	}

	if _, err := os.Stat(localProfileFile); err == nil {
		return loadFile(localProfileFile)
	}

	if xdgPath, ok := xdgConfigPath(); ok {
		if _, err := os.Stat(xdgPath); err == nil {
			return loadFile(xdgPath)
		}
	}

	return Preset()
}

// loadFile reads and loads a profile from an explicitly selected path.
func loadFile(path string) (*Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading profile %q: %w", path, err)
	}
	p, err := Load(data)
	if err != nil {
		return nil, fmt.Errorf("loading profile %q: %w", path, err)
	}
	return p, nil
}

// xdgConfigPath returns the XDG-style config path for somm's profile file:
// $XDG_CONFIG_HOME/somm/somm.yaml, falling back to ~/.config/somm/somm.yaml
// when XDG_CONFIG_HOME is unset. ok is false only when neither the env var
// nor the user's home directory can be determined.
func xdgConfigPath() (string, bool) {
	if base := os.Getenv("XDG_CONFIG_HOME"); base != "" {
		return filepath.Join(base, "somm", localProfileFile), true
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	return filepath.Join(home, ".config", "somm", localProfileFile), true
}
