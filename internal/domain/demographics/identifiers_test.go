// internal/domain/demographics/identifiers_test.go
package demographics

import "testing"

func TestIsValidCPF(t *testing.T) {
	tests := []struct {
		value string
		valid bool
	}{
		{value: "52998224725", valid: true},
		{value: "529.982.247-25", valid: true},
		{value: "12345678909", valid: true},
		{value: "52998224724", valid: false},
		{value: "12345678901", valid: false},
		{value: "00000000000", valid: false},
		{value: "111.111.111-11", valid: false},
		{value: "123", valid: false},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			if got := IsValidCPF(tt.value); got != tt.valid {
				t.Fatalf("IsValidCPF(%q) = %v, want %v", tt.value, got, tt.valid)
			}
		})
	}
}

func TestIsValidCNS(t *testing.T) {
	tests := []struct {
		value string
		valid bool
	}{
		{value: "174598435280018", valid: true},
		{value: "174 5984 3528 0018", valid: true},
		{value: "700000000000005", valid: true},
		{value: "174598435280019", valid: false},
		{value: "123456789012345", valid: false},
		{value: "000000000000000", valid: false},
		{value: "123", valid: false},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			if got := IsValidCNS(tt.value); got != tt.valid {
				t.Fatalf("IsValidCNS(%q) = %v, want %v", tt.value, got, tt.valid)
			}
		})
	}
}
