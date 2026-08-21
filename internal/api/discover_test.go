package api

import (
	"strconv"
	"strings"
	"testing"
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
