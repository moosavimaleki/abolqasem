package server

import "testing"

func TestWorkspaceMachineNameUsesMacComputerName(t *testing.T) {
	withWorkspaceMachineMocks(t, "darwin", "host.local", "My Mac\n", nil)

	if got := workspaceMachineName(); got != "My Mac" {
		t.Fatalf("workspaceMachineName() = %q, expected My Mac", got)
	}
}

func TestWorkspaceMachineNameStripsLocalNetworkSuffix(t *testing.T) {
	withWorkspaceMachineMocks(t, "linux", "dev-box.local", "", errMockCommand)

	if got := workspaceMachineName(); got != "dev-box" {
		t.Fatalf("workspaceMachineName() = %q, expected dev-box", got)
	}

	withWorkspaceMachineMocks(t, "linux", "office.LAN", "", errMockCommand)
	if got := workspaceMachineName(); got != "office" {
		t.Fatalf("workspaceMachineName() = %q, expected office", got)
	}
}

func TestWorkspacePlatformMatchesAbolqasemNodePlatform(t *testing.T) {
	withWorkspaceMachineMocks(t, "windows", "host", "", errMockCommand)
	if got := workspacePlatform(); got != "win32" {
		t.Fatalf("workspacePlatform() = %q, expected win32", got)
	}

	withWorkspaceMachineMocks(t, "linux", "host", "", errMockCommand)
	if got := workspacePlatform(); got != "linux" {
		t.Fatalf("workspacePlatform() = %q, expected linux", got)
	}
}

func withWorkspaceMachineMocks(t *testing.T, platform string, hostname string, commandOutput string, commandErr error) {
	t.Helper()
	previousPlatform := workspaceMachinePlatform
	previousHostname := workspaceMachineHostname
	previousCommandOutput := workspaceMachineCommandOutput
	workspaceMachinePlatform = func() string { return platform }
	workspaceMachineHostname = func() (string, error) { return hostname, nil }
	workspaceMachineCommandOutput = func(string, ...string) (string, error) { return commandOutput, commandErr }
	t.Cleanup(func() {
		workspaceMachinePlatform = previousPlatform
		workspaceMachineHostname = previousHostname
		workspaceMachineCommandOutput = previousCommandOutput
	})
}

var errMockCommand = mockMachineError{}

type mockMachineError struct{}

func (mockMachineError) Error() string {
	return "mock command failed"
}
