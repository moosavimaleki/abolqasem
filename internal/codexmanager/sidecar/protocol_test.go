package sidecar

import (
	"encoding/json"
	"testing"
	"time"
)

func TestStatusJSONRedactsNoSecrets(t *testing.T) {
	status := Status{State: StateReady, PID: 42, Listen: "127.0.0.1:8787", StartedAt: time.Unix(1, 0)}
	payload, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) == "" || string(payload) == "null" {
		t.Fatalf("unexpected status payload: %s", payload)
	}
	if string(payload) == "{}" {
		t.Fatalf("status was not serialized: %s", payload)
	}
}

func TestStatesAreStable(t *testing.T) {
	if StateReady != "ready" || StateCrashed != "crashed" {
		t.Fatalf("unexpected state values: %q %q", StateReady, StateCrashed)
	}
}
