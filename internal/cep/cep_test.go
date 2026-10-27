package cep

import "testing"

func TestValid(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"eight digits", "29902555", true},
		{"leading zeros", "00000000", true},
		{"too short", "2990255", false},
		{"too long", "299025555", false},
		{"with hyphen", "2990-2555", false},
		{"letters", "abcdefgh", false},
		{"empty", "", false},
		{"spaces", "29902 55", false},
		{"unicode digits", "２９９０２５５５", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Valid(tt.in); got != tt.want {
				t.Errorf("Valid(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
