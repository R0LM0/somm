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

func TestBrightenHex(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"dark color lightens toward white", "#2a1e30", "#837c86"},
		{"pure black lightens to gray", "#000000", "#6b6b6b"},
		{"pure white stays white", "#ffffff", "#ffffff"},
		{"empty string passes through unchanged", "", ""},
		{"malformed hex passes through unchanged", "#zzz", "#zzz"},
		{"missing leading hash passes through unchanged", "2a1e30", "2a1e30"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := brightenHex(tt.in); got != tt.want {
				t.Errorf("brightenHex(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
