package plans

import "testing"

// TestOpenCodeZenGo_KnownModelResolvesRequests locks in plan-quota-currency
// spec's "Table resolves a known model id" scenario: a known ocId in the
// embedded table returns its requests_per_5h with ok=true.
func TestOpenCodeZenGo_KnownModelResolvesRequests(t *testing.T) {
	table, err := OpenCodeZenGo()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, ok := table.Requests("deepseek-v4-flash")
	if !ok {
		t.Fatal("expected known ocId \"deepseek-v4-flash\" to resolve, got ok=false")
	}
	if got != 5000 {
		t.Errorf("requests_per_5h = %d, want 5000", got)
	}
}

// TestOpenCodeZenGo_UnknownModelReturnsNotFound locks in plan-quota-currency
// spec's "Table omits an unknown model id" scenario: absence MUST report
// ok=false, never a zero-as-default.
func TestOpenCodeZenGo_UnknownModelReturnsNotFound(t *testing.T) {
	table, err := OpenCodeZenGo()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, ok := table.Requests("does-not-exist-anywhere")
	if ok {
		t.Errorf("expected ok=false for unknown ocId, got ok=true (value %d)", got)
	}
}

// TestOpenCodeZenGo_MeasuredAtParsesAsISODate asserts the table's
// measured_at date is present and validated as 2006-01-02 at load time.
func TestOpenCodeZenGo_MeasuredAtParsesAsISODate(t *testing.T) {
	table, err := OpenCodeZenGo()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if table.MeasuredAt != "2026-08-01" {
		t.Errorf("MeasuredAt = %q, want %q", table.MeasuredAt, "2026-08-01")
	}
}

// TestTable_RequestsOnHandBuiltTable exercises Requests() directly on a
// hand-built Table (no embed dependency), confirming the ok=false contract
// generalizes beyond the specific embedded fixture.
func TestTable_RequestsOnHandBuiltTable(t *testing.T) {
	tb := &Table{
		Plan:       "test-plan",
		MeasuredAt: "2026-01-01",
		Models:     map[string]int{"model-a": 110},
	}

	got, ok := tb.Requests("model-a")
	if !ok || got != 110 {
		t.Errorf("Requests(model-a) = (%d, %v), want (110, true)", got, ok)
	}

	_, ok = tb.Requests("model-b")
	if ok {
		t.Error("Requests(model-b) = ok=true, want ok=false (absent from table)")
	}
}
