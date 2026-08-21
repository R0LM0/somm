package api

import (
	"context"
	"fmt"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestParseDiscoverOutput_Valid covers a multi-provider fixture matching the
// live-verified `opencode models --verbose` shape: a `providerID/id` label
// line followed by a JSON object per model, no enclosing array or NDJSON
// envelope (design D1, D7). Every field maps to the correct DiscoveredModel;
// prices stay USD-per-1M (no conversion at this layer — that happens at
// merge time, design D7).
func TestParseDiscoverOutput_Valid(t *testing.T) {
	raw := []byte(`opencode-go/grok-4.5
{"id":"grok-4.5","providerID":"opencode-go","name":"Grok 4.5","cost":{"input":2,"output":10,"cache":{"read":0.5,"write":2}},"limit":{"context":200000}}
kimi-for-coding/kimi-k2
{"id":"kimi-k2","providerID":"kimi-for-coding","name":"Kimi K2","cost":{"input":0.6,"output":2.5}}
`)

	got, err := parseDiscoverOutput(raw)
	if err != nil {
		t.Fatalf("parseDiscoverOutput() error = %v, want nil", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}

	first := got[0]
	if first.ProviderID != "opencode-go" || first.ID != "grok-4.5" || first.Name != "Grok 4.5" {
		t.Errorf("first record identity = %+v, want opencode-go/grok-4.5 \"Grok 4.5\"", first)
	}
	if first.InputPerM != 2 || first.OutputPerM != 10 {
		t.Errorf("first record price = input:%v output:%v, want input:2 output:10", first.InputPerM, first.OutputPerM)
	}
	if first.CacheReadPerM == nil || *first.CacheReadPerM != 0.5 {
		t.Errorf("first record CacheReadPerM = %v, want 0.5", first.CacheReadPerM)
	}
	if first.ContextLength == nil || *first.ContextLength != 200000 {
		t.Errorf("first record ContextLength = %v, want 200000", first.ContextLength)
	}

	second := got[1]
	if second.ProviderID != "kimi-for-coding" || second.ID != "kimi-k2" {
		t.Errorf("second record identity = %+v, want kimi-for-coding/kimi-k2", second)
	}
	if second.InputPerM != 0.6 || second.OutputPerM != 2.5 {
		t.Errorf("second record price = input:%v output:%v, want input:0.6 output:2.5", second.InputPerM, second.OutputPerM)
	}
	if second.CacheReadPerM != nil {
		t.Errorf("second record CacheReadPerM = %v, want nil (no cache object in fixture)", second.CacheReadPerM)
	}
}

// TestParseDiscoverOutput_PrettyPrintedMultilineObject covers the ACTUAL
// shape `opencode models --verbose` prints on this machine (multi-line,
// 2-space-indented JSON, not compact single-line) — verified live against
// `opencode models opencode-go --verbose`. This is the case
// TestParseDiscoverOutput_Valid's compact fixture does not exercise: brace
// counting across many lines, not just one.
func TestParseDiscoverOutput_PrettyPrintedMultilineObject(t *testing.T) {
	raw := []byte(`opencode-go/grok-4.5
{
  "id": "grok-4.5",
  "providerID": "opencode-go",
  "name": "Grok 4.5",
  "api": {
    "id": "grok-4.5",
    "url": "https://opencode.ai/zen/go/v1"
  },
  "cost": {
    "input": 2,
    "output": 6,
    "cache": {
      "read": 0.3,
      "write": 0
    }
  },
  "limit": {
    "context": 200000
  }
}
opencode-go/gpt-5.6-luna
{
  "id": "gpt-5.6-luna",
  "providerID": "opencode-go",
  "name": "GPT 5.6 Luna",
  "cost": {
    "input": 0.2,
    "output": 1.2
  }
}
`)

	got, err := parseDiscoverOutput(raw)
	if err != nil {
		t.Fatalf("parseDiscoverOutput() error = %v, want nil", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].ID != "grok-4.5" || got[0].InputPerM != 2 {
		t.Errorf("first record = %+v, want id=grok-4.5 input=2", got[0])
	}
	if got[0].CacheReadPerM == nil || *got[0].CacheReadPerM != 0.3 {
		t.Errorf("first record CacheReadPerM = %v, want 0.3", got[0].CacheReadPerM)
	}
	if got[1].ID != "gpt-5.6-luna" || got[1].InputPerM != 0.2 {
		t.Errorf("second record = %+v, want id=gpt-5.6-luna input=0.2", got[1])
	}
}

// TestParseDiscoverOutput_NoRecognizableObject covers stdout that contains no
// line starting with `{` at all (e.g. a future CLI version emitting
// human-only text, or a version-mismatch error banner). There is no envelope
// to validate against in this format, so this yields an empty result, not an
// error — the caller (execDiscoverer) still gets a clean "nothing
// discovered" and degrades to the existing catalog (design D4).
func TestParseDiscoverOutput_NoRecognizableObject(t *testing.T) {
	got, err := parseDiscoverOutput([]byte("not json at all, just prose\nno braces on any line\n"))
	if err != nil {
		t.Fatalf("parseDiscoverOutput() error = %v, want nil (no envelope to fail)", err)
	}
	if len(got) != 0 {
		t.Fatalf("len(got) = %d, want 0", len(got))
	}
}

// TestParseDiscoverOutput_TruncatedObjectErrors covers stdout that cuts off
// mid-record (process killed, pipe closed early) — an object that starts but
// never returns to brace-depth zero. This is the one case that must surface
// as a distinct error, not a silent per-entry drop (design D4).
func TestParseDiscoverOutput_TruncatedObjectErrors(t *testing.T) {
	raw := []byte(`opencode-go/grok-4.5
{
  "id": "grok-4.5",
  "providerID": "opencode-go",
  "cost": {
    "input": 2,
`)
	got, err := parseDiscoverOutput(raw)
	if err == nil {
		t.Fatal("parseDiscoverOutput() error = nil, want non-nil for a truncated object")
	}
	if got != nil {
		t.Errorf("got = %v, want nil result alongside the error", got)
	}
}

// TestParseDiscoverOutput_MissingIDOrProviderID covers design D4's one
// exception: a per-entry malformed record is dropped individually, siblings
// are retained, and the whole parse does not fail.
func TestParseDiscoverOutput_MissingIDOrProviderID(t *testing.T) {
	raw := []byte(`opencode-go/missing-id
{"providerID":"opencode-go","name":"Missing ID","cost":{"input":1,"output":2}}
missing-provider
{"id":"missing-provider","name":"Missing ProviderID","cost":{"input":1,"output":2}}
openai/gpt-5.6
{"id":"gpt-5.6","providerID":"openai","name":"GPT 5.6","cost":{"input":3,"output":12}}
`)

	got, err := parseDiscoverOutput(raw)
	if err != nil {
		t.Fatalf("parseDiscoverOutput() error = %v, want nil (per-entry drop, not whole-parse failure)", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1 (two malformed entries dropped)", len(got))
	}
	if got[0].ID != "gpt-5.6" || got[0].ProviderID != "openai" {
		t.Errorf("surviving record = %+v, want openai/gpt-5.6", got[0])
	}
}

// TestParseDiscoverOutput_NullCost covers a null "cost" field — dropped
// individually, siblings retained (design D4).
func TestParseDiscoverOutput_NullCost(t *testing.T) {
	raw := []byte(`openai/no-price
{"id":"no-price","providerID":"openai","name":"No Price","cost":null}
openai/gpt-5.6
{"id":"gpt-5.6","providerID":"openai","name":"GPT 5.6","cost":{"input":3,"output":12}}
`)

	got, err := parseDiscoverOutput(raw)
	if err != nil {
		t.Fatalf("parseDiscoverOutput() error = %v, want nil (per-entry drop)", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1 (null-cost entry dropped)", len(got))
	}
	if got[0].ID != "gpt-5.6" {
		t.Errorf("surviving record ID = %q, want gpt-5.6", got[0].ID)
	}
}

// TestParseDiscoverOutput_UnknownExtraFieldsIgnored is the PATH-hijack
// threat-matrix case at the parse layer: hostile/unexpected fields (nested
// objects, arrays, oversized strings) must decode-and-ignore, never fail the
// parse and never leak into DiscoveredModel.
func TestParseDiscoverOutput_UnknownExtraFieldsIgnored(t *testing.T) {
	raw := []byte(`openai/gpt-5.6
{
	"id":"gpt-5.6","providerID":"openai","name":"GPT 5.6",
	"cost":{"input":3,"output":12,"unexpected_cost_field":"ignored"},
	"limit":{"context":128000,"weird_nested":{"a":[1,2,3]}},
	"path":"/usr/bin/sh","exec":["rm","-rf","/"],"toolCall":{"nested":{"deep":true}}
}
`)

	got, err := parseDiscoverOutput(raw)
	if err != nil {
		t.Fatalf("parseDiscoverOutput() error = %v, want nil (unknown fields must not fail parse)", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].ID != "gpt-5.6" || got[0].ProviderID != "openai" {
		t.Errorf("record identity = %+v, want openai/gpt-5.6", got[0])
	}
	if got[0].InputPerM != 3 || got[0].OutputPerM != 12 {
		t.Errorf("record price = input:%v output:%v, want input:3 output:12", got[0].InputPerM, got[0].OutputPerM)
	}
	if got[0].ContextLength == nil || *got[0].ContextLength != 128000 {
		t.Errorf("record ContextLength = %v, want 128000", got[0].ContextLength)
	}
}

// TestParseDiscoverOutput_LargeValidInputDoesNotChoke confirms the pure
// parser handles a large-but-valid input without choking. The 8 MiB
// io.LimitReader guard is Phase 3's subprocess-boundary concern (design
// tasks 3.3); this only proves the parser itself scales.
func TestParseDiscoverOutput_LargeValidInputDoesNotChoke(t *testing.T) {
	const n = 5000
	var sb strings.Builder
	for i := 0; i < n; i++ {
		sb.WriteString("openai/model-" + strconv.Itoa(i) + "\n")
		sb.WriteString(`{"id":"model-` + strconv.Itoa(i) + `","providerID":"openai","name":"Model","cost":{"input":1,"output":2}}` + "\n")
	}

	got, err := parseDiscoverOutput([]byte(sb.String()))
	if err != nil {
		t.Fatalf("parseDiscoverOutput() error = %v, want nil", err)
	}
	if len(got) != n {
		t.Fatalf("len(got) = %d, want %d", len(got), n)
	}
	if got[n-1].ID != "model-"+strconv.Itoa(n-1) {
		t.Errorf("last record ID = %q, want %q", got[n-1].ID, "model-"+strconv.Itoa(n-1))
	}
}

// --- execDiscoverer: subprocess boundary (Phase 3, threat matrix) ---
//
// The production runner is replaced by an injected `runner func(ctx)
// ([]byte, error)` in every test below (design Testing Strategy:
// Unit(subprocess)) — none of these spawn a real `opencode` process, so they
// stay fast and portable. TestExecDiscoverer_ArgvExactNoInterpolation is the
// one exception: it asserts the pure argv-builder used by the REAL runner
// directly, which is what actually proves "no shell, no interpolation"
// without spawning anything.

// TestExecDiscoverer_ArgvExactNoInterpolation is the threat-matrix "binary
// resolution" case: the real runner's argv must be exactly
// {"models","--verbose"}, with the only variation being the fixed
// "--refresh" literal gated by SOMM_OC_DISCOVERY_REFRESH=1 — never any
// interpolated/user-supplied value.
func TestExecDiscoverer_ArgvExactNoInterpolation(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		got := buildDiscoverArgv()
		want := []string{"models", "--verbose"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("buildDiscoverArgv() = %v, want %v", got, want)
		}
	})

	t.Run("refresh escape hatch", func(t *testing.T) {
		t.Setenv("SOMM_OC_DISCOVERY_REFRESH", "1")
		got := buildDiscoverArgv()
		want := []string{"models", "--verbose", "--refresh"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("buildDiscoverArgv() = %v, want %v", got, want)
		}
	})

	t.Run("refresh unset leaves argv untouched", func(t *testing.T) {
		t.Setenv("SOMM_OC_DISCOVERY_REFRESH", "0")
		got := buildDiscoverArgv()
		want := []string{"models", "--verbose"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("buildDiscoverArgv() = %v, want %v", got, want)
		}
	})
}

// TestExecDiscoverer_HostileOutputIgnoresUnknownFields is the threat-matrix
// "PATH hijack" case: a hostile runner (standing in for a compromised
// `opencode` on PATH) emits unexpected/nested fields and path-like/exec-like
// values. Discover must decode into the typed DiscoveredModel and never
// execute or otherwise act on the hostile content — it reuses the
// already-tested parseDiscoverOutput.
func TestExecDiscoverer_HostileOutputIgnoresUnknownFields(t *testing.T) {
	hostile := []byte(`openai/gpt-5.6
{"id":"gpt-5.6","providerID":"openai","name":"GPT 5.6","cost":{"input":3,"output":12},"path":"/usr/bin/sh","exec":["rm","-rf","/"],"nested":{"a":{"b":{"c":1}}}}
`)
	d := newExecDiscoverer()
	d.runner = func(ctx context.Context) ([]byte, error) { return hostile, nil }

	got, err := d.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover() error = %v, want nil", err)
	}
	if len(got) != 1 || got[0].ID != "gpt-5.6" || got[0].ProviderID != "openai" {
		t.Fatalf("Discover() = %+v, want one openai/gpt-5.6 record with unknown fields ignored", got)
	}
	if got[0].InputPerM != 3 || got[0].OutputPerM != 12 {
		t.Errorf("Discover() price = input:%v output:%v, want input:3 output:12", got[0].InputPerM, got[0].OutputPerM)
	}
}

// TestExecDiscoverer_OversizeStdoutTreatedAsMalformed is the threat-matrix
// "unbounded stdout" case: output past the 8 MiB cap is treated as
// malformed — Discover warns and returns (nil, error) non-fatally, it never
// crashes and never attempts to parse the oversized payload.
func TestExecDiscoverer_OversizeStdoutTreatedAsMalformed(t *testing.T) {
	oversized := make([]byte, discoverMaxStdout+1)
	for i := range oversized {
		oversized[i] = 'a'
	}

	d := newExecDiscoverer()
	d.runner = func(ctx context.Context) ([]byte, error) { return oversized, nil }

	got, err := d.Discover(context.Background())
	if err == nil {
		t.Fatal("Discover() error = nil, want non-nil for oversize stdout")
	}
	if got != nil {
		t.Errorf("Discover() = %v, want nil result alongside the error", got)
	}
}

// TestExecDiscoverer_HangPastDeadlineKilled is the threat-matrix
// "hang/no exit" case: a runner that never returns on its own must still
// bound Discover's total wait to the configured timeout — proving the
// context deadline (which exec.CommandContext ties process kill/reap to in
// the real runner) actually propagates into the runner call.
func TestExecDiscoverer_HangPastDeadlineKilled(t *testing.T) {
	d := newExecDiscoverer()
	d.timeout = 30 * time.Millisecond
	d.runner = func(ctx context.Context) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}

	start := time.Now()
	got, err := d.Discover(context.Background())
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Discover() error = nil, want non-nil for a hung runner")
	}
	if got != nil {
		t.Errorf("Discover() = %v, want nil result alongside the error", got)
	}
	if elapsed > time.Second {
		t.Fatalf("Discover() took %v, want bounded near the 30ms timeout (proves it does not wait forever)", elapsed)
	}
}

// TestExecDiscoverer_NonZeroExitStderrTruncated is the threat-matrix
// "non-zero exit" case: stderr must be captured but never logged/propagated
// in full — truncateStderr (used by the real runner) bounds it to
// discoverMaxStderr bytes.
func TestExecDiscoverer_NonZeroExitStderrTruncated(t *testing.T) {
	longStderr := strings.Repeat("x", discoverMaxStderr*4)
	truncated := truncateStderr([]byte(longStderr), discoverMaxStderr)
	if len(truncated) >= len(longStderr) {
		t.Fatalf("truncateStderr() did not shrink a %d-byte input", len(longStderr))
	}
	if !strings.HasPrefix(truncated, strings.Repeat("x", discoverMaxStderr)) {
		t.Fatalf("truncateStderr() = %q, want it to start with the first %d bytes", truncated, discoverMaxStderr)
	}

	d := newExecDiscoverer()
	d.runner = func(ctx context.Context) ([]byte, error) {
		return nil, fmt.Errorf("opencode discovery: process exited with error: exit status 1 (stderr: %s)", truncated)
	}

	got, err := d.Discover(context.Background())
	if err == nil {
		t.Fatal("Discover() error = nil, want non-nil for a non-zero exit")
	}
	if got != nil {
		t.Errorf("Discover() = %v, want nil result alongside the error", got)
	}
	if strings.Contains(err.Error(), longStderr) {
		t.Error("Discover() error contains the full untruncated stderr, want it bounded")
	}
}

// TestExecDiscoverer_ConcurrentCallsSingleFlight is the threat-matrix
// "concurrent tool calls" case: N callers racing Discover() must share one
// subprocess invocation, never spawn N. The counting runner is the proof.
func TestExecDiscoverer_ConcurrentCallsSingleFlight(t *testing.T) {
	var calls int32
	release := make(chan struct{})

	d := newExecDiscoverer()
	d.runner = func(ctx context.Context) ([]byte, error) {
		atomic.AddInt32(&calls, 1)
		<-release // hold the "subprocess" open until every caller has joined
		return []byte(`openai/gpt-5.6
{"id":"gpt-5.6","providerID":"openai","name":"GPT 5.6","cost":{"input":3,"output":12}}
`), nil
	}

	const n = 20
	var wg sync.WaitGroup
	results := make([][]DiscoveredModel, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = d.Discover(context.Background())
		}(i)
	}

	// Give every goroutine a chance to reach the in-flight wait before
	// releasing the single shared "subprocess" call.
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("runner invocation count = %d, want exactly 1 (single-flight)", got)
	}
	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Errorf("Discover() call %d error = %v, want nil", i, errs[i])
		}
		if len(results[i]) != 1 || results[i][0].ID != "gpt-5.6" {
			t.Errorf("Discover() call %d = %+v, want the shared gpt-5.6 result", i, results[i])
		}
	}
}

// TestExecDiscoverer_TTLCache proves success is cached longer than failure
// (design D3: success 5m, failure 60s) and that an expired entry re-invokes
// the runner. cachedAt is rewound directly (white-box, same package) instead
// of sleeping out real minutes.
func TestExecDiscoverer_TTLCache(t *testing.T) {
	var calls int32
	d := newExecDiscoverer()
	d.runner = func(ctx context.Context) ([]byte, error) {
		atomic.AddInt32(&calls, 1)
		return []byte(`openai/gpt-5.6
{"id":"gpt-5.6","providerID":"openai","name":"GPT 5.6","cost":{"input":3,"output":12}}
`), nil
	}

	if _, err := d.Discover(context.Background()); err != nil {
		t.Fatalf("Discover() error = %v, want nil", err)
	}
	if _, err := d.Discover(context.Background()); err != nil {
		t.Fatalf("Discover() error = %v, want nil", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("runner invocation count = %d, want 1 (second call within success TTL must hit the cache)", got)
	}

	d.mu.Lock()
	d.cachedAt = time.Now().Add(-discoverSuccessTTL - time.Second)
	d.mu.Unlock()

	if _, err := d.Discover(context.Background()); err != nil {
		t.Fatalf("Discover() error = %v, want nil", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("runner invocation count = %d, want 2 (success TTL expired, must re-invoke)", got)
	}

	// Now exercise the shorter failure TTL: force a failing call, confirm
	// the cached failure is reused immediately, then confirm it expires
	// faster than a cached success would.
	failing := &execDiscoverer{runner: func(ctx context.Context) ([]byte, error) {
		atomic.AddInt32(&calls, 1)
		return nil, errFakeDiscover
	}, timeout: discoverTimeout}

	if _, err := failing.Discover(context.Background()); err == nil {
		t.Fatal("Discover() error = nil, want non-nil for the injected failure")
	}
	before := atomic.LoadInt32(&calls)
	if _, err := failing.Discover(context.Background()); err == nil {
		t.Fatal("Discover() error = nil, want non-nil (still within failure TTL)")
	}
	if got := atomic.LoadInt32(&calls); got != before {
		t.Fatalf("runner invocation count changed from %d to %d, want unchanged (cached failure within TTL)", before, got)
	}

	failing.mu.Lock()
	failing.cachedAt = time.Now().Add(-discoverFailureTTL - time.Second)
	failing.mu.Unlock()

	if _, err := failing.Discover(context.Background()); err == nil {
		t.Fatal("Discover() error = nil, want non-nil for the injected failure")
	}
	if got := atomic.LoadInt32(&calls); got != before+1 {
		t.Fatalf("runner invocation count = %d, want %d (failure TTL expired, must re-invoke)", got, before+1)
	}
}

// TestExecDiscoverer_DisabledByEnv proves SOMM_DISCOVERY=off is the rollback
// path: Discover must never invoke the runner at all.
func TestExecDiscoverer_DisabledByEnv(t *testing.T) {
	t.Setenv("SOMM_DISCOVERY", "off")

	var calls int32
	d := newExecDiscoverer()
	d.runner = func(ctx context.Context) ([]byte, error) {
		atomic.AddInt32(&calls, 1)
		return []byte(`openai/gpt-5.6
{"id":"gpt-5.6","providerID":"openai","name":"GPT 5.6","cost":{"input":3,"output":12}}
`), nil
	}

	got, err := d.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover() error = %v, want nil", err)
	}
	if got != nil {
		t.Errorf("Discover() = %v, want nil (disabled)", got)
	}
	if calls != 0 {
		t.Fatalf("runner invocation count = %d, want 0 (SOMM_DISCOVERY=off must never invoke the runner)", calls)
	}
}

// errFakeDiscover is a sentinel error for TTL-cache tests that need a
// deterministic non-nil failure without depending on runOpencodeDiscover.
var errFakeDiscover = fmt.Errorf("fake discover failure")

// TestExecDiscoverer_RealCLI exercises the real opencode CLI end to end. It
// skips in -short mode and when opencode is not on PATH (design Testing
// Strategy: Integration), and asserts a sane $/M range as a D7 unit-error
// regression guard.
func TestExecDiscoverer_RealCLI(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real opencode CLI integration test in -short mode")
	}
	if _, err := exec.LookPath("opencode"); err != nil {
		t.Skip("opencode not found on PATH")
	}

	d := newExecDiscoverer()
	models, err := d.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover() error = %v, want nil against a real opencode install", err)
	}
	for _, m := range models {
		if m.InputPerM < 0 || m.InputPerM > 1000 {
			t.Errorf("model %s/%s InputPerM = %v, outside sane $/M range (D7 unit regression guard)", m.ProviderID, m.ID, m.InputPerM)
		}
		if m.OutputPerM < 0 || m.OutputPerM > 1000 {
			t.Errorf("model %s/%s OutputPerM = %v, outside sane $/M range (D7 unit regression guard)", m.ProviderID, m.ID, m.OutputPerM)
		}
	}
}
