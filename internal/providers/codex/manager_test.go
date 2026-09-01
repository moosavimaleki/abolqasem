package codex

import (
	"context"
	"errors"
	"testing"

	codexprotocol "abolqasem/internal/providers/codex/protocol"
)

func TestStartSessionInitializesAndStartsFreshThread(t *testing.T) {
	client := &fakeRPCClient{
		handler: func(call rpcCall) error {
			switch call.Method {
			case "initialize":
				params := call.Params.(codexprotocol.InitializeParams)
				if !params.Capabilities.ExperimentalAPI {
					t.Fatal("expected experimentalApi capability")
				}
				return nil
			case "thread/start":
				result := call.Result.(*codexprotocol.ThreadStartResponse)
				result.Thread.ID = "thread-1"
				result.Model = "gpt-5.4"
				return nil
			default:
				t.Fatalf("unexpected method %s", call.Method)
			}
			return nil
		},
	}
	manager := NewManager(client)

	token, err := manager.StartSession(context.Background(), StartSessionArgs{
		ChatID: "chat-1",
		CWD:    "/tmp/project",
		Model:  "gpt-5.4",
	})
	if err != nil {
		t.Fatalf("StartSession returned error: %v", err)
	}
	if token != "thread-1" {
		t.Fatalf("expected thread-1, got %q", token)
	}
	if got := methodNames(client.calls); !equalStrings(got, []string{"initialize", "thread/start"}) {
		t.Fatalf("unexpected calls: %#v", got)
	}
	session, ok := manager.Session("chat-1")
	if !ok || session.ThreadID != "thread-1" || session.CWD != "/tmp/project" {
		t.Fatalf("unexpected session: %#v", session)
	}
}

func TestStartSessionFallsBackWhenResumeIsRecoverablyMissing(t *testing.T) {
	client := &fakeRPCClient{
		handler: func(call rpcCall) error {
			switch call.Method {
			case "initialize":
				return nil
			case "thread/resume":
				return errors.New("thread/resume failed: thread not found")
			case "thread/start":
				result := call.Result.(*codexprotocol.ThreadStartResponse)
				result.Thread.ID = "thread-2"
				result.Model = "gpt-5.4"
				return nil
			default:
				t.Fatalf("unexpected method %s", call.Method)
			}
			return nil
		},
	}
	manager := NewManager(client)

	token, err := manager.StartSession(context.Background(), StartSessionArgs{
		ChatID:       "chat-1",
		CWD:          "/tmp/project",
		Model:        "gpt-5.4",
		SessionToken: "missing-thread",
	})
	if err != nil {
		t.Fatalf("StartSession returned error: %v", err)
	}
	if token != "thread-2" {
		t.Fatalf("expected thread-2, got %q", token)
	}
	if got := methodNames(client.calls); !equalStrings(got, []string{"initialize", "thread/resume", "thread/start"}) {
		t.Fatalf("unexpected calls: %#v", got)
	}
}

func TestStartSessionRecoversLegacyCompletedSubagentItem(t *testing.T) {
	client := &fakeRPCClient{handler: func(call rpcCall) error {
		switch call.Method {
		case "initialize":
			return nil
		case "thread/resume":
			return errors.New("json-rpc error -32603: failed to deserialize stored thread item subagent-completed-1: unknown variant `completed`")
		case "thread/start":
			result := call.Result.(*codexprotocol.ThreadStartResponse)
			result.Thread.ID = "fresh-thread"
			result.Model = "gpt-5.6"
			return nil
		default:
			t.Fatalf("unexpected method %s", call.Method)
		}
		return nil
	}}
	manager := NewManager(client)
	token, err := manager.StartSession(context.Background(), StartSessionArgs{ChatID: "chat-1", CWD: "/tmp/project", SessionToken: "legacy-thread"})
	if err != nil || token != "fresh-thread" {
		t.Fatalf("expected fresh recovery, token=%q err=%v", token, err)
	}
}

func TestStartSessionForksPendingForkToken(t *testing.T) {
	client := &fakeRPCClient{
		handler: func(call rpcCall) error {
			switch call.Method {
			case "initialize":
				return nil
			case "thread/fork":
				params := call.Params.(codexprotocol.ThreadForkParams)
				if params.ThreadID != "thread-source" {
					t.Fatalf("expected source thread, got %q", params.ThreadID)
				}
				result := call.Result.(*codexprotocol.ThreadForkResponse)
				result.Thread.ID = "thread-fork-1"
				result.Model = "gpt-5.4"
				return nil
			default:
				t.Fatalf("unexpected method %s", call.Method)
			}
			return nil
		},
	}
	manager := NewManager(client)

	token, err := manager.StartSession(context.Background(), StartSessionArgs{
		ChatID:                  "chat-1",
		CWD:                     "/tmp/project",
		Model:                   "gpt-5.4",
		PendingForkSessionToken: "thread-source",
	})
	if err != nil {
		t.Fatalf("StartSession returned error: %v", err)
	}
	if token != "thread-fork-1" {
		t.Fatalf("expected fork token, got %q", token)
	}
	if got := methodNames(client.calls); !equalStrings(got, []string{"initialize", "thread/fork"}) {
		t.Fatalf("unexpected calls: %#v", got)
	}
}

type rpcCall struct {
	Method string
	Params any
	Result any
}

type fakeRPCClient struct {
	calls   []rpcCall
	handler func(rpcCall) error
}

func (c *fakeRPCClient) Call(_ context.Context, method string, params any, result any) error {
	call := rpcCall{Method: method, Params: params, Result: result}
	c.calls = append(c.calls, call)
	if c.handler == nil {
		return nil
	}
	return c.handler(call)
}

func methodNames(calls []rpcCall) []string {
	names := make([]string, 0, len(calls))
	for _, call := range calls {
		names = append(names, call.Method)
	}
	return names
}

func equalStrings(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
