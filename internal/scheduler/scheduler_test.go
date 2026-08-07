package scheduler

import (
	"testing"
)

func TestCleanMarkdownForTelegram(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"**Bold** text", "Bold text"},
		{"*Italic* text", "Italic text"},
		{"_Underline_ text", "Underline text"},
		{"[[Link]]", "Link"},
		{"#Tag", "Tag"},
		{"**Bold** and [[Link]] with #Tag", "Bold and Link with Tag"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := cleanMarkdownForTelegram(tt.input)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}
