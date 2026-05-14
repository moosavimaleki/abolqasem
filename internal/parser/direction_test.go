package parser

import "testing"

func TestDetectDirection(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Pure Persian",
			input:    "سلام این یک متن فارسی است",
			expected: "rtl",
		},
		{
			name:     "Pure English",
			input:    "Hello this is English",
			expected: "ltr",
		},
		{
			name:     "Mixed Persian Dominant",
			input:    "سلام این یک test است",
			expected: "rtl",
		},
		{
			name:     "Mixed English Dominant",
			input:    "Hello this is a تست",
			expected: "ltr",
		},
		{
			name:     "Code block",
			input:    "func main() { fmt.Println(\"سلام\") }",
			expected: "ltr",
		},
		{
			name:     "Path",
			input:    "/home/user/project/test.go",
			expected: "ltr",
		},
		{
			name:     "Empty",
			input:    "",
			expected: "auto",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DetectDirection(tt.input); got != tt.expected {
				t.Errorf("DetectDirection() = %v, want %v", got, tt.expected)
			}
		})
	}
}
