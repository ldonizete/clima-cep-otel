package weathersvc

import "testing"

func TestConversions(t *testing.T) {
	tests := []struct {
		c, wantF, wantK float64
	}{
		{28.5, 83.3, 301.5},
		{0, 32, 273},
		{-10, 14, 263},
		{100, 212, 373},
		{21.7, 71.06, 294.7},
	}
	for _, tt := range tests {
		if got := ToFahrenheit(tt.c); got != tt.wantF {
			t.Errorf("ToFahrenheit(%v) = %v, want %v", tt.c, got, tt.wantF)
		}
		if got := ToKelvin(tt.c); got != tt.wantK {
			t.Errorf("ToKelvin(%v) = %v, want %v", tt.c, got, tt.wantK)
		}
	}
}
