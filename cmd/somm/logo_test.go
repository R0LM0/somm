package main

import "testing"

// TestSommLogoData_RowCountsMatch guards against the exact bug this test
// was added for: sommLogoData.go is hand-copied generated data (see
// tools/img2braille), and a transcription slip that drops one row of
// sommLogoColors relative to sommLogoLines panicked renderLogo() with an
// index-out-of-range on every "somm setup" invocation.
func TestSommLogoData_RowCountsMatch(t *testing.T) {
	if len(sommLogoLines) != len(sommLogoColors) {
		t.Fatalf("sommLogoLines has %d rows, sommLogoColors has %d — must match",
			len(sommLogoLines), len(sommLogoColors))
	}
}

func TestRenderLogo_DoesNotPanic(t *testing.T) {
	_ = renderLogo()
}
