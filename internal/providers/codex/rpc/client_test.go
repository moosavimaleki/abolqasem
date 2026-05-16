package rpc

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

func TestRoutesResponsesToMatchingPendingCalls(t *testing.T) {
	transport := &fakeTransport{}
	client := NewClient(transport)

	firstDone := make(chan string, 1)
	secondDone := make(chan string, 1)
	go func() {
		var result struct {
			Value string `json:"value"`
		}
		if err := client.Call(context.Background(), "first", map[string]string{"x": "1"}, &result); err != nil {
			firstDone <- "error:" + err.Error()
			return
		}
		firstDone <- result.Value
	}()
	go func() {
		var result struct {
			Value string `json:"value"`
		}
		if err := client.Call(context.Background(), "second", map[string]string{"x": "2"}, &result); err != nil {
			secondDone <- "error:" + err.Error()
			return
		}
		secondDone <- result.Value
	}()

	transport.waitForMessages(t, 2)
	ids := transport.methodIDs(t)
	if err := client.HandleMessage([]byte(`{"id":"` + ids["second"] + `","result":{"value":"two"}}`)); err != nil {
		t.Fatalf("HandleMessage returned error: %v", err)
	}
	if err := client.HandleMessage([]byte(`{"id":"` + ids["first"] + `","result":{"value":"one"}}`)); err != nil {
		t.Fatalf("HandleMessage returned error: %v", err)
	}

	if got := <-firstDone; got != "one" {
		t.Fatalf("expected first response, got %q", got)
	}
	if got := <-secondDone; got != "two" {
		t.Fatalf("expected second response, got %q", got)
	}
}

func TestStreamsNotifications(t *testing.T) {
	client := NewClient(&fakeTransport{})
	if err := client.HandleMessage([]byte(`{"method":"turn/completed","params":{"threadId":"thread-1"}}`)); err != nil {
		t.Fatalf("HandleMessage returned error: %v", err)
	}

	select {
	case notification := <-client.Notifications():
		if notification.Method != "turn/completed" {
			t.Fatalf("unexpected notification: %#v", notification)
		}
		var params struct {
			ThreadID string `json:"threadId"`
		}
		if err := json.Unmarshal(notification.Params, &params); err != nil {
			t.Fatalf("Unmarshal params returned error: %v", err)
		}
		if params.ThreadID != "thread-1" {
			t.Fatalf("expected thread-1, got %q", params.ThreadID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for notification")
	}
}

func TestRecordsStderr(t *testing.T) {
	client := NewClient(&fakeTransport{})
	client.RecordStderr("fatal: app-server crashed")
	if got := client.Stderr(); got != "fatal: app-server crashed\n" {
		t.Fatalf("unexpected stderr log: %q", got)
	}
}

type fakeTransport struct {
	mu       sync.Mutex
	messages [][]byte
}

func (t *fakeTransport) Send(message []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.messages = append(t.messages, append([]byte(nil), message...))
	return nil
}

func (t *fakeTransport) waitForMessages(tb testing.TB, count int) {
	tb.Helper()
	deadline := time.Now().Add(time.Second)
	for t.messageCount() < count {
		if time.Now().After(deadline) {
			tb.Fatalf("timed out waiting for %d messages, got %d", count, t.messageCount())
		}
		time.Sleep(time.Millisecond)
	}
}

func (t *fakeTransport) messageCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.messages)
}

func (t *fakeTransport) methodIDs(tb testing.TB) map[string]string {
	tb.Helper()
	t.mu.Lock()
	defer t.mu.Unlock()

	ids := map[string]string{}
	for _, message := range t.messages {
		var envelope struct {
			ID     string `json:"id"`
			Method string `json:"method"`
		}
		if err := json.Unmarshal(message, &envelope); err != nil {
			tb.Fatalf("Unmarshal request returned error: %v", err)
		}
		ids[envelope.Method] = envelope.ID
	}
	return ids
}
