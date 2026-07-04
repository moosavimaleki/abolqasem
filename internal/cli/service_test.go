package cli

import (
	"reflect"
	"testing"
)

func TestServiceCommandArgsDefaultToAutoPort(t *testing.T) {
	t.Setenv("ABOLQASEM_SERVICE_PORT", "")
	t.Setenv("ABOLQASEM_DEV_PORT", "")
	t.Setenv("AI_AGENT_MANAGER_SERVICE_PORT", "")
	t.Setenv("AI_AGENT_MANAGER_DEV_PORT", "")

	got := serviceCommandArgs()
	want := []string{"__server", "--auto-port"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("serviceCommandArgs() = %#v, want %#v", got, want)
	}
}

func TestServiceCommandArgsUseConfiguredPort(t *testing.T) {
	t.Setenv("ABOLQASEM_SERVICE_PORT", "9092")

	got := serviceCommandArgs()
	want := []string{"__server", "--port", "9092"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("serviceCommandArgs() = %#v, want %#v", got, want)
	}
}
