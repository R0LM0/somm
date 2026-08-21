package profile

import "testing"

// TestFrequencyWeight locks in plan-quota-currency spec's Frequency
// Weighting mapping: high -> 4.0, medium (and "" unset) -> 1.0, low -> 0.25.
func TestFrequencyWeight(t *testing.T) {
	tests := []struct {
		freq string
		want float64
	}{
		{FrequencyHigh, 4.0},
		{FrequencyMedium, 1.0},
		{"", 1.0}, // unset defaults to medium's weight
		{FrequencyLow, 0.25},
	}

	for _, tt := range tests {
		t.Run(tt.freq, func(t *testing.T) {
			got := FrequencyWeight(tt.freq)
			if got != tt.want {
				t.Errorf("FrequencyWeight(%q) = %v, want %v", tt.freq, got, tt.want)
			}
		})
	}
}
