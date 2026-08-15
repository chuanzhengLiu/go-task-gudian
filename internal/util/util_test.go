package util

import "testing"

func TestValidatePassword(t *testing.T) {
	cases := []struct {
		name     string
		password string
		want     bool
	}{
		{"valid", "Abcdef1234", true},
		{"valid with symbols", "Abcdef1234!@#", true},
		{"too short", "Abcd12345", false},
		{"no digit", "Abcdefghijk", false},
		{"no uppercase", "abcdefg1234", false},
		{"no lowercase", "ABCDEFG1234", false},
		{"empty", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ValidatePassword(tc.password); got != tc.want {
				t.Errorf("ValidatePassword(%q) = %v, want %v", tc.password, got, tc.want)
			}
		})
	}
}
