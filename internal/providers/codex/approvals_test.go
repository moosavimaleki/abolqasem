package codex

import (
	"encoding/json"
	"testing"
)

func TestHandleUserInputRequest(t *testing.T) {
	params := mustJSON(map[string]any{
		"threadId": "thread-1",
		"turnId":   "turn-1",
		"itemId":   "req-1",
		"questions": []map[string]any{{
			"id":       "favorite_color",
			"header":   "Color",
			"question": "What is your favorite color right now?",
			"options": []map[string]any{{
				"label": "Red",
			}},
		}},
	})

	events, response, err := HandleServerRequest(ServerRequest{
		ID:     "request-1",
		Method: "item/tool/requestUserInput",
		Params: params,
	}, RequestHandlers{
		OnToolRequest: func(request ToolRequest) (map[string]any, error) {
			toolID := request.Tool["toolId"]
			if toolID != "req-1" {
				t.Fatalf("expected tool id req-1, got %#v", toolID)
			}
			return map[string]any{
				"answers": map[string]any{
					"What is your favorite color right now?": "Red",
				},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("HandleServerRequest returned error: %v", err)
	}
	if len(events) != 1 || events[0].Entry["kind"] != "tool_call" {
		t.Fatalf("unexpected events: %#v", events)
	}
	answers := response.Result["answers"].(map[string]any)
	answer := answers["favorite_color"].(map[string]any)
	values := answer["answers"].([]string)
	if len(values) != 1 || values[0] != "Red" {
		t.Fatalf("unexpected answer response: %#v", response)
	}
}

func TestHandleApprovalRequest(t *testing.T) {
	events, response, err := HandleServerRequest(ServerRequest{
		ID:     "approval-1",
		Method: "item/commandExecution/requestApproval",
		Params: mustJSON(map[string]any{
			"threadId": "thread-1",
			"turnId":   "turn-1",
			"itemId":   "call-1",
			"command":  "rm -rf .",
			"cwd":      "/tmp/project",
		}),
	}, RequestHandlers{
		OnApprovalRequest: func(request ApprovalRequest) (string, error) {
			if request.Kind != "command_execution" {
				t.Fatalf("expected command_execution, got %q", request.Kind)
			}
			if request.Tool["toolKind"] != "approval_request" || request.Tool["toolId"] != "approval-1" {
				t.Fatalf("unexpected approval tool: %#v", request.Tool)
			}
			input, _ := request.Tool["input"].(map[string]any)
			if input["command"] != "rm -rf ." || input["cwd"] != "/tmp/project" {
				t.Fatalf("approval preview was not preserved: %#v", input)
			}
			return "accept", nil
		},
	})
	if err != nil {
		t.Fatalf("HandleServerRequest returned error: %v", err)
	}
	if response.ID != "approval-1" || response.Result["decision"] != "accept" {
		t.Fatalf("unexpected response: %#v", response)
	}
	if len(events) != 1 || events[0].Entry["kind"] != "tool_call" {
		t.Fatalf("expected approval tool transcript event, got %#v", events)
	}
}

func TestHandleApprovalRejectsUnknownDecision(t *testing.T) {
	_, response, err := HandleServerRequest(ServerRequest{
		ID:     "approval-invalid",
		Method: "item/fileChange/requestApproval",
		Params: mustJSON(map[string]any{"itemId": "file-1"}),
	}, RequestHandlers{
		OnApprovalRequest: func(ApprovalRequest) (string, error) { return "anything", nil },
	})
	if err != nil {
		t.Fatalf("HandleServerRequest returned error: %v", err)
	}
	if response.Result["decision"] != "decline" {
		t.Fatalf("expected fail-closed decline, got %#v", response)
	}
}

func TestHandleCommandApprovalDefaultsToDecline(t *testing.T) {
	_, response, err := HandleServerRequest(ServerRequest{
		ID:     "approval-default",
		Method: "item/commandExecution/requestApproval",
		Params: mustJSON(map[string]any{
			"threadId": "thread-1",
			"turnId":   "turn-1",
			"itemId":   "call-1",
			"command":  "rm -rf .",
			"cwd":      "/tmp/project",
		}),
	}, RequestHandlers{})
	if err != nil {
		t.Fatalf("HandleServerRequest returned error: %v", err)
	}
	if response.Result["decision"] != "decline" {
		t.Fatalf("expected decline, got %#v", response)
	}
}

func TestHandleFileChangeApprovalDefaultsToDecline(t *testing.T) {
	_, response, err := HandleServerRequest(ServerRequest{
		ID:     "approval-2",
		Method: "item/fileChange/requestApproval",
		Params: mustJSON(map[string]any{
			"threadId": "thread-1",
			"turnId":   "turn-1",
			"itemId":   "file-1",
		}),
	}, RequestHandlers{})
	if err != nil {
		t.Fatalf("HandleServerRequest returned error: %v", err)
	}
	if response.Result["decision"] != "decline" {
		t.Fatalf("expected decline, got %#v", response)
	}
}

func mustJSON(value any) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return data
}
