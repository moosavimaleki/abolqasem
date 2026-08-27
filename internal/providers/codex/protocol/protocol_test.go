package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestUserInputMarshalMatchesAppServerUnion(t *testing.T) {
	payload, err := json.Marshal([]UserInput{{Type: "text", Text: "hello"}, {Type: "localImage", Path: "/tmp/shot.png"}})
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(payload)
	if !strings.Contains(encoded, `"text_elements":[]`) {
		t.Fatalf("text input must include required text_elements: %s", encoded)
	}
	if !strings.Contains(encoded, `{"type":"localImage","path":"/tmp/shot.png"}`) {
		t.Fatalf("unexpected localImage input: %s", encoded)
	}
	if strings.Contains(encoded, `"localImage","text"`) {
		t.Fatalf("localImage must not contain text-only fields: %s", encoded)
	}
}

func TestThreadForkParamsSupportsCheckpointTurnBoundary(t *testing.T) {
	turnID := "turn-checkpoint"
	payload, err := json.Marshal(ThreadForkParams{ThreadID: "thread-source", BeforeTurnID: &turnID})
	if err != nil {
		t.Fatal(err)
	}
	if encoded := string(payload); !strings.Contains(encoded, `"beforeTurnId":"turn-checkpoint"`) {
		t.Fatalf("checkpoint fork must retain beforeTurnId: %s", encoded)
	}
}
