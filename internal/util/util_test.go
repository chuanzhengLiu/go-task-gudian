package util

import "testing"

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		want     bool
	}{
		{"valid - all three classes", "Abcdefgh1k", true},
		{"missing number (regression)", "Abcdefghijk", false},
		{"missing uppercase", "abcdefgh1k", false},
		{"missing lowercase", "ABCDEFGH1K", false},
		{"too short", "Ab1defg", false},
		{"exactly 10 with all classes", "Abcdefgh1j", true},
		{"empty", "", false},
		{"number only", "1234567890", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidatePassword(tt.password); got != tt.want {
				t.Errorf("ValidatePassword(%q) = %v, want %v", tt.password, got, tt.want)
			}
		})
	}
}
