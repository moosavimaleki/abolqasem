package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

type SSEEvent struct {
	EventKey    string `json:"event_key"`
	SessionKey  string `json:"session_key"`
	SessionID   string `json:"session_id"`
	ProjectName string `json:"project_name"`
	UpdatedAt   string `json:"updated_at"`
}

type Broker struct {
	clients map[chan []byte]bool
	mu      sync.Mutex
}

var EventBroker = &Broker{
	clients: make(map[chan []byte]bool),
}

func (b *Broker) AddClient(client chan []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.clients[client] = true
}

func (b *Broker) RemoveClient(client chan []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.clients, client)
	close(client)
}

func (b *Broker) Broadcast(event SSEEvent) {
	data, _ := json.Marshal(event)
	idLine := ""
	if event.EventKey != "" {
		idLine = fmt.Sprintf("id: %s\n", event.EventKey)
	}
	msg := []byte(fmt.Sprintf("%sdata: %s\n\n", idLine, string(data)))

	b.mu.Lock()
	defer b.mu.Unlock()
	for client := range b.clients {
		select {
		case client <- msg:
		default:
			// Drop blocked clients
		}
	}
}

func handleAPIEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	clientChan := make(chan []byte, 10)
	EventBroker.AddClient(clientChan)
	defer EventBroker.RemoveClient(clientChan)

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-clientChan:
			w.Write(msg)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}
}
