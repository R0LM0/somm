package api

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestFileReferencePricer_Lookup is a fixture table mirroring
// discover_test.go's style: one fixed reference catalog, exercised against
// every distinguishable outcome (valid match, case-insensitive provider
// match, missing provider, missing model, zero cost in the reference file
// itself, malformed JSON, empty file, and an absent file altogether).
func TestFileReferencePricer_Lookup(t *testing.T) {
	validJSON := `{
		"openai": {
			"models": {
				"gpt-4o": {"cost": {"input": 2.5, "output": 10}},
				"gpt-4o-mini": {"cost": {"input": 0, "output": 0}}
			}
		}
	}`

	tests := []struct {
		name           string
		fileContent    string
		skipFile       bool
		providerID     string
		slug           string
		wantInputPerM  float64
		wantOutputPerM float64
		wantOK         bool
	}{
		{
			name:           "valid match",
			fileContent:    validJSON,
			providerID:     "openai",
			slug:           "gpt-4o",
			wantInputPerM:  2.5,
			wantOutputPerM: 10,
			wantOK:         true,
		},
		{
			name:           "case-insensitive provider match",
			fileContent:    validJSON,
			providerID:     "OpenAI",
			slug:           "gpt-4o",
			wantInputPerM:  2.5,
			wantOutputPerM: 10,
			wantOK:         true,
		},
		{
			name:        "missing provider",
			fileContent: validJSON,
			providerID:  "anthropic",
			slug:        "claude",
			wantOK:      false,
		},
		{
			name:        "missing model",
			fileContent: validJSON,
			providerID:  "openai",
			slug:        "gpt-9",
			wantOK:      false,
		},
		{
			name:        "zero cost in reference file too",
			fileContent: validJSON,
			providerID:  "openai",
			slug:        "gpt-4o-mini",
			wantOK:      false,
		},
		{
			name:        "malformed JSON",
			fileContent: `{"openai": not valid json`,
			providerID:  "openai",
			slug:        "gpt-4o",
			wantOK:      false,
		},
		{
			name:        "empty file",
			fileContent: "",
			providerID:  "openai",
			slug:        "gpt-4o",
			wantOK:      false,
		},
		{
			name:       "file absent",
			skipFile:   true,
			providerID: "openai",
			slug:       "gpt-4o",
			wantOK:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "models.json")
			if !tt.skipFile {
				if err := os.WriteFile(path, []byte(tt.fileContent), 0o644); err != nil {
					t.Fatalf("writing fixture: %v", err)
				}
			}

			p := newFileReferencePricer()
			p.pathFn = func() (string, error) { return path, nil }

			gotInput, gotOutput, gotOK := p.Lookup(tt.providerID, tt.slug)
			if gotOK != tt.wantOK {
				t.Fatalf("Lookup() ok = %v, want %v", gotOK, tt.wantOK)
			}
			if !gotOK {
				return
			}
			if gotInput != tt.wantInputPerM || gotOutput != tt.wantOutputPerM {
				t.Errorf("Lookup() = (%v, %v), want (%v, %v)", gotInput, gotOutput, tt.wantInputPerM, tt.wantOutputPerM)
			}
		})
	}
}

// TestFileReferencePricer_PerEntryMalformedRecordSkippedNotFatal covers
// design D4's per-entry drop spirit: a sibling model entry with a
// wrong-typed cost field must not prevent a well-formed sibling from being
// found in the same file.
func TestFileReferencePricer_PerEntryMalformedRecordSkippedNotFatal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "models.json")
	content := `{
		"openai": {
			"models": {
				"bad-model": {"cost": {"input": "not-a-number", "output": 10}},
				"gpt-4o": {"cost": {"input": 2.5, "output": 10}}
			}
		}
	}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	p := newFileReferencePricer()
	p.pathFn = func() (string, error) { return path, nil }

	input, output, ok := p.Lookup("openai", "gpt-4o")
	if !ok || input != 2.5 || output != 10 {
		t.Fatalf("Lookup(gpt-4o) = (%v, %v, %v), want (2.5, 10, true) despite a malformed sibling entry", input, output, ok)
	}

	if _, _, ok := p.Lookup("openai", "bad-model"); ok {
		t.Error("Lookup(bad-model) ok = true, want false for the malformed cost.input")
	}
}

// TestFileReferencePricer_OversizeFileTreatedAsMalformed proves the
// io.LimitReader guard: a file past the configured cap is treated as
// malformed (never partially parsed), mirroring discover.go's
// discoverMaxStdout guard. maxFileSize is overridden so the test fixture
// stays tiny instead of writing a real 32 MiB file.
func TestFileReferencePricer_OversizeFileTreatedAsMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "models.json")
	content := `{"openai":{"models":{"gpt-4o":{"cost":{"input":2.5,"output":10}}}}}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	p := newFileReferencePricer()
	p.pathFn = func() (string, error) { return path, nil }
	p.maxFileSize = 10 // force the fixture above to exceed the cap

	if _, _, ok := p.Lookup("openai", "gpt-4o"); ok {
		t.Fatal("Lookup() ok = true, want false for an oversize file")
	}
}

// TestFileReferencePricer_WarnsOnceNotPerLookup covers the D4-style failure
// taxonomy: a missing/malformed catalog must warn exactly once per cache
// refresh, never once per Lookup call.
func TestFileReferencePricer_WarnsOnceNotPerLookup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.json") // never created

	var buf bytes.Buffer
	prevLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(prevLogger)

	p := newFileReferencePricer()
	p.pathFn = func() (string, error) { return path, nil }

	for i := 0; i < 5; i++ {
		if _, _, ok := p.Lookup("openai", "gpt-4o"); ok {
			t.Fatal("Lookup() ok = true, want false for an absent file")
		}
	}

	got := strings.Count(buf.String(), "reference price catalog unavailable")
	if got != 1 {
		t.Fatalf("warn count = %d, want exactly 1 (warned once, not per Lookup call): log=%s", got, buf.String())
	}
}

// TestFileReferencePricer_TTLCache mirrors TestExecDiscoverer_TTLCache
// (discover_test.go): a cached success is reused within TTL, and an expired
// entry re-reads the file. cachedAt is rewound directly (white-box, same
// package) instead of sleeping out the real TTL, and pathFn's own call count
// is the load-invocation proof.
func TestFileReferencePricer_TTLCache(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "models.json")
	content := `{"openai":{"models":{"gpt-4o":{"cost":{"input":2.5,"output":10}}}}}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	var calls int32
	p := newFileReferencePricer()
	p.pathFn = func() (string, error) {
		atomic.AddInt32(&calls, 1)
		return path, nil
	}

	if _, _, ok := p.Lookup("openai", "gpt-4o"); !ok {
		t.Fatal("Lookup() ok = false, want true")
	}
	if _, _, ok := p.Lookup("openai", "gpt-4o"); !ok {
		t.Fatal("Lookup() ok = false, want true")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("pathFn call count = %d, want 1 (second Lookup within TTL must hit the cache)", got)
	}

	p.mu.Lock()
	p.cachedAt = time.Now().Add(-discoverSuccessTTL - time.Second)
	p.mu.Unlock()

	if _, _, ok := p.Lookup("openai", "gpt-4o"); !ok {
		t.Fatal("Lookup() ok = false, want true")
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("pathFn call count = %d, want 2 (success TTL expired, must re-load)", got)
	}
}

// TestClient_ReferencePricer_DefaultsToNoopUnderTest mirrors discover_test's
// treatment of Client.discoverer(): under `go test`, a Client without an
// explicit ReferencePricer must resolve to a safe no-op, never touching the
// real filesystem/home directory.
func TestClient_ReferencePricer_DefaultsToNoopUnderTest(t *testing.T) {
	c := &Client{}
	if _, _, ok := c.referencePricer().Lookup("openai", "gpt-4o"); ok {
		t.Fatal("referencePricer() under test = a matching pricer, want the hermetic no-op default")
	}
}

// TestClient_ReferencePricer_ExplicitInjectionWins proves an explicitly-set
// Client.ReferencePricer always wins over the test-time no-op default.
func TestClient_ReferencePricer_ExplicitInjectionWins(t *testing.T) {
	c := &Client{ReferencePricer: mapReferencePricer{"openai/gpt-4o": [2]float64{2.5, 10}}}
	input, output, ok := c.referencePricer().Lookup("openai", "gpt-4o")
	if !ok || input != 2.5 || output != 10 {
		t.Fatalf("referencePricer().Lookup() = (%v, %v, %v), want (2.5, 10, true)", input, output, ok)
	}
}
