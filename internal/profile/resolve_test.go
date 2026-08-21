package profile

import (
	"os"
	"path/filepath"
	"testing"
)

// withCwd temporarily changes the working directory for the duration of a
// test and restores it afterward.
func withCwd(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(orig); err != nil {
			t.Fatalf("restoring cwd: %v", err)
		}
	})
}

func writeProfile(t *testing.T, path, id string) {
	t.Helper()
	content := "version: 1\nroles:\n  - id: " + id + "\n    weights:\n      intelligence: 1.0\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

func TestResolve_FlagWinsOverEnvAndFile(t *testing.T) {
	dir := t.TempDir()
	withCwd(t, dir)

	t.Setenv(envVar, filepath.Join(dir, "env-profile.yaml"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))

	writeProfile(t, filepath.Join(dir, "env-profile.yaml"), "from-env")
	writeProfile(t, filepath.Join(dir, localProfileFile), "from-local")

	flagPath := filepath.Join(dir, "flag-profile.yaml")
	writeProfile(t, flagPath, "from-flag")

	p, err := Resolve(flagPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.Roles) != 1 || p.Roles[0].ID != "from-flag" {
		t.Fatalf("expected role from-flag, got %+v", p.Roles)
	}
}

func TestResolve_NoExternalSourceFallsThroughToEmbedded(t *testing.T) {
	dir := t.TempDir()
	withCwd(t, dir)

	t.Setenv(envVar, "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg-empty"))

	p, err := Resolve("")
	if err != nil {
		t.Fatalf("expected no error falling through to embedded default, got: %v", err)
	}
	if len(p.Roles) != 19 {
		t.Fatalf("expected embedded gentle-ai preset with 19 roles, got %d", len(p.Roles))
	}
}

func TestTierCurrency(t *testing.T) {
	tests := []struct {
		name    string
		tier    string
		want    string
		wantErr bool
	}{
		{"empty tier defaults to usd", "", CurrencyUSD, false},
		{"go tier resolves to quota", "go", CurrencyQuota, false},
		{"zen tier resolves to quota", "zen", CurrencyQuota, false},
		{"unrecognized tier errors", "bogus", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := TierCurrency(tt.tier)
			if (err != nil) != tt.wantErr {
				t.Fatalf("TierCurrency(%q) error = %v, wantErr %v", tt.tier, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("TierCurrency(%q) = %q, want %q", tt.tier, got, tt.want)
			}
		})
	}
}

func TestResolveWithTier_ExplicitCurrencyBeatsTier(t *testing.T) {
	dir := t.TempDir()
	withCwd(t, dir)
	t.Setenv(envVar, "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg-empty"))

	flagPath := filepath.Join(dir, "explicit.yaml")
	content := "version: 1\nselection:\n  currency: usd\nroles:\n  - id: r1\n    weights:\n      intelligence: 1.0\n"
	if err := os.WriteFile(flagPath, []byte(content), 0o644); err != nil {
		t.Fatalf("writing profile: %v", err)
	}

	p, err := ResolveWithTier(flagPath, "go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Roles[0].Selection.Currency != CurrencyUSD {
		t.Fatalf("currency = %q, want %q (explicit selection.currency must beat a non-empty tier)", p.Roles[0].Selection.Currency, CurrencyUSD)
	}
}

func TestResolveWithTier_TierAppliesWhenCurrencyNotExplicit(t *testing.T) {
	dir := t.TempDir()
	withCwd(t, dir)
	t.Setenv(envVar, "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg-empty"))

	flagPath := filepath.Join(dir, "implicit.yaml")
	writeProfile(t, flagPath, "r1")

	p, err := ResolveWithTier(flagPath, "zen")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Roles[0].Selection.Currency != CurrencyQuota {
		t.Fatalf("currency = %q, want %q (tier must apply when currency wasn't explicit)", p.Roles[0].Selection.Currency, CurrencyQuota)
	}
}

func TestResolveWithTier_UnrecognizedTierFailsLoud(t *testing.T) {
	dir := t.TempDir()
	withCwd(t, dir)
	t.Setenv(envVar, "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg-empty"))

	if _, err := ResolveWithTier("", "bogus"); err == nil {
		t.Fatal("expected error for unrecognized SOMM_OC_TIER value, got nil")
	}
}

func TestResolveIsResolveWithTierEmptyTier(t *testing.T) {
	dir := t.TempDir()
	withCwd(t, dir)
	t.Setenv(envVar, "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg-empty"))

	flagPath := filepath.Join(dir, "flag-profile.yaml")
	writeProfile(t, flagPath, "from-flag")

	viaResolve, err := Resolve(flagPath)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	viaResolveWithTier, err := ResolveWithTier(flagPath, "")
	if err != nil {
		t.Fatalf("ResolveWithTier: %v", err)
	}
	if viaResolve.Roles[0].Selection.Currency != viaResolveWithTier.Roles[0].Selection.Currency {
		t.Fatalf("Resolve(p) currency %q != ResolveWithTier(p, \"\") currency %q", viaResolve.Roles[0].Selection.Currency, viaResolveWithTier.Roles[0].Selection.Currency)
	}
}

func TestResolve_MalformedProfileFailsLoud(t *testing.T) {
	dir := t.TempDir()
	withCwd(t, dir)

	t.Setenv(envVar, "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg-empty"))

	if err := os.WriteFile(filepath.Join(dir, localProfileFile), []byte("roles: [not: valid: yaml"), 0o644); err != nil {
		t.Fatalf("writing malformed profile: %v", err)
	}

	_, err := Resolve("")
	if err == nil {
		t.Fatal("expected error for malformed ./somm.yaml, got nil")
	}
}
