package workspace

import "testing"

func TestSanitizeIdentifier(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"ABC-123", "ABC-123"},
		{"foo/bar", "foo_bar"},
		{"a b", "a_b"},
		{"café", "caf_"},
		{"MT.123", "MT.123"},
		{"simple_name", "simple_name"},
		{"has@special#chars!", "has_special_chars_"},
		{"../traversal", ".._traversal"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := SanitizeIdentifier(tt.input)
			if got != tt.expected {
				t.Errorf("SanitizeIdentifier(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestValidateWorkspacePath_InsideRoot(t *testing.T) {
	err := ValidateWorkspacePath("/tmp/ws/MT-123", "/tmp/ws")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestValidateWorkspacePath_OutsideRoot(t *testing.T) {
	err := ValidateWorkspacePath("/other/path", "/tmp/ws")
	if err == nil {
		t.Error("expected error for path outside root")
	}
}

func TestValidateWorkspacePath_Traversal(t *testing.T) {
	err := ValidateWorkspacePath("/tmp/ws/../secret", "/tmp/ws")
	if err == nil {
		t.Error("expected error for path traversal")
	}
}

func TestValidateWorkspacePath_RootItself(t *testing.T) {
	err := ValidateWorkspacePath("/tmp/ws", "/tmp/ws")
	if err == nil {
		t.Error("expected error when path equals root")
	}
}
