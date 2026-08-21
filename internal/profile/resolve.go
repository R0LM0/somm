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
//
// Resolve(flagPath) is exactly ResolveWithTier(flagPath, "").
func Resolve(flagPath string) (*Profile, error) {
	return ResolveWithTier(flagPath, "")
}

// TierCurrency maps a persisted SOMM_OC_TIER value to its effective default
// currency: an unset tier ("") defaults to usd (today's behavior), "go" and
// "zen" both resolve to quota (the tier is stored as-is, not collapsed to a
// boolean, so a future per-tier table split needs no re-prompt), and any
// other value is a fatal error — never a silent fallback (role-profiles
// spec, design Decision 5).
func TierCurrency(tier string) (string, error) {
	switch tier {
	case "":
		return CurrencyUSD, nil
	case "go", "zen":
		return CurrencyQuota, nil
	default:
		return "", fmt.Errorf("unrecognized SOMM_OC_TIER value %q (valid: go, zen)", tier)
	}
}

// ApplyTierCurrency overwrites the effective currency of every role whose
// currency was NOT set explicitly (role- or profile-level
// selection.currency) with the given currency. Roles with an explicit
// currency are left untouched — an explicit selection.currency always beats
// the tier (role-profiles spec, Requirement: Per-Role Selection Override;
// design Decision 5).
func ApplyTierCurrency(p *Profile, currency string) {
	for i := range p.Roles {
		r := &p.Roles[i]
		if !r.currencyExplicit {
			r.Selection.Currency = currency
		}
	}
}

// ResolveWithTier resolves the active Profile the same way resolveSource
// does, then refines it with the OpenCode tier captured by the setup
// wizard: TierCurrency derives the tier's effective currency (fatal on an
// unrecognized tier), and ApplyTierCurrency overwrites only the roles whose
// currency was not set explicitly. Load always yields a complete profile
// first, so the tier only ever refines an already-usable state — it never
// leaves a half-resolved profile (design Decision 5).
func ResolveWithTier(flagPath, tier string) (*Profile, error) {
	p, err := resolveSource(flagPath)
	if err != nil {
		return nil, err
	}
	currency, err := TierCurrency(tier)
	if err != nil {
		return nil, err
	}
	ApplyTierCurrency(p, currency)
	return p, nil
}

// resolveSource is Resolve's original source-selection logic, unexported so
// both Resolve and ResolveWithTier can share it.
func resolveSource(flagPath string) (*Profile, error) {
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
