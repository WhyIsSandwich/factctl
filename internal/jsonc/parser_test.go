package jsonc

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]interface{}
		wantErr  bool
	}{
		{
			name: "basic json with line comments",
			input: `{
				// This is a comment
				"foo": "bar", // End of line comment
				"baz": 123
			}`,
			expected: map[string]interface{}{
				"foo": "bar",
				"baz": float64(123),
			},
		},
		{
			name: "json with block comments",
			input: `{
				/* Block comment */
				"foo": "bar",
				/* Multi-line
				   block comment */
				"baz": 123
			}`,
			expected: map[string]interface{}{
				"foo": "bar",
				"baz": float64(123),
			},
		},
		{
			name: "mixed comments",
			input: `{
				// Line comment
				"foo": /* inline block */ "bar",
				/* Block
				   comment */ "baz": 123 // EOL comment
			}`,
			expected: map[string]interface{}{
				"foo": "bar",
				"baz": float64(123),
			},
		},
		{
			name:    "invalid json",
			input:   `{"foo": }`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got map[string]interface{}
			err := Parse(strings.NewReader(tt.input), &got)
			
			if tt.wantErr {
				if err == nil {
					t.Error("Parse() expected error but got nil")
				}
				return
			}
			
			if err != nil {
				t.Errorf("Parse() error = %v", err)
				return
			}

			if len(got) != len(tt.expected) {
				t.Errorf("Parse() got %v entries, expected %v", len(got), len(tt.expected))
				return
			}

			for k, v := range tt.expected {
				if gv, ok := got[k]; !ok || gv != v {
					t.Errorf("Parse() for key %q got = %v, expected %v", k, gv, v)
				}
			}
		})
	}
}