package proxy

import (
	"testing"
)

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://example.com:443/v1", "https://example.com/v1"},
		{"https://example.com:443", "https://example.com"},
		{"http://example.com:80/v1", "http://example.com/v1"},
		{"http://example.com:80", "http://example.com"},
		{"https://example.com:8443/v1", "https://example.com:8443/v1"},
		{"https://example.com/v1", "https://example.com/v1"},
		{"not a url", "not a url"},
	}

	for _, tc := range tests {
		got := NormalizeURL(tc.input)
		if got != tc.expected {
			t.Errorf("NormalizeURL(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}
