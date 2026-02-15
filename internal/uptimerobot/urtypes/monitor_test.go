package urtypes

import "testing"

func TestMonitorTypeFromAPIString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected MonitorType
	}{
		{
			name:     "http",
			input:    APITypeHTTP,
			expected: TypeHTTPS,
		},
		{
			name:     "api maps to https",
			input:    APITypeAPI,
			expected: TypeHTTPS,
		},
		{
			name:     "dns",
			input:    APITypeDNS,
			expected: TypeDNS,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MonitorTypeFromAPIString(tt.input)
			if got != tt.expected {
				t.Fatalf("expected %v, got %v", tt.expected, got)
			}
		})
	}
}
