package codexutil

import "testing"

func TestExtractExecCommand(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		wantCommand   string
		wantDirectory string
		wantOK        bool
	}{
		{
			name:          "single command",
			input:         `tools.exec_command({"cmd":"go test ./...","workdir":"/repo"})`,
			wantCommand:   "go test ./...",
			wantDirectory: "/repo",
			wantOK:        true,
		},
		{
			name:          "multiple commands with common directory",
			input:         `tools.exec_command({"cmd":"git status","workdir":"/repo"}) tools.exec_command({"cmd":"go test ./...","workdir":"/repo"})`,
			wantCommand:   "git status\ngo test ./...",
			wantDirectory: "/repo",
			wantOK:        true,
		},
		{
			name:        "multiple directories",
			input:       `tools.exec_command({"cmd":"git status","workdir":"/one"}) tools.exec_command({"cmd":"go test ./...","workdir":"/two"})`,
			wantCommand: "git status\ngo test ./...",
			wantOK:      true,
		},
		{name: "not an exec command", input: `tools.read_file({"path":"x"})`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command, directory, ok := ExtractExecCommand(test.input)
			if command != test.wantCommand || directory != test.wantDirectory || ok != test.wantOK {
				t.Fatalf("ExtractExecCommand() = (%q, %q, %t), want (%q, %q, %t)", command, directory, ok, test.wantCommand, test.wantDirectory, test.wantOK)
			}
		})
	}
}

func TestCommandCompletion(t *testing.T) {
	tests := []struct {
		output     string
		wantStatus string
		wantCode   int
		wantKnown  bool
	}{
		{"Process exited with code 0", "completed", 0, true},
		{"exit code: 42", "failed", 42, true},
		{"Script completed", "completed", 0, true},
		{"still running", "completed", 0, false},
	}
	for _, test := range tests {
		status, code, known := CommandCompletion(test.output)
		if status != test.wantStatus || code != test.wantCode || known != test.wantKnown {
			t.Fatalf("CommandCompletion(%q) = (%q, %d, %t), want (%q, %d, %t)", test.output, status, code, known, test.wantStatus, test.wantCode, test.wantKnown)
		}
	}
}
