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
