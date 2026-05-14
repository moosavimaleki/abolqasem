package parser

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

type Message struct {
	ID        string     `json:"id"`
	SessionID string     `json:"session_id"`
	Role      string     `json:"role"`
	Kind      string     `json:"kind"`
	Text      string     `json:"text"`
	Direction string     `json:"direction"`
	Index     int        `json:"index"`
	CreatedAt *time.Time `json:"created_at"`
}

type ParseResult struct {
	SessionID     string    `json:"session_id"`
	Items         []Message `json:"items"`
	HasMoreBefore bool      `json:"has_more_before"`
	HasMoreAfter  bool      `json:"has_more_after"`
	OldestCursor  string    `json:"oldest_cursor"`
	NewestCursor  string    `json:"newest_cursor"`
}

func ParseMessages(sessionID, filepath string, limit int, beforeIndex int) (*ParseResult, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var allMessages []Message
	scanner := bufio.NewScanner(file)
	index := 0

	for scanner.Scan() {
		line := scanner.Bytes()
		index++
		
		var raw map[string]interface{}
		if err := json.Unmarshal(line, &raw); err != nil {
			continue // skip invalid lines
		}
		
		msg := extractMessage(raw, sessionID, index)
		if msg != nil {
			allMessages = append(allMessages, *msg)
		}
	}

	result := &ParseResult{
		SessionID: sessionID,
		Items:     []Message{},
	}

	if len(allMessages) == 0 {
		return result, nil
	}

	// Simple pagination logic
	endIdx := len(allMessages)
	if beforeIndex > 0 {
		for i, m := range allMessages {
			if m.Index == beforeIndex {
				endIdx = i
				break
			}
		}
	}

	startIdx := endIdx - limit
	if startIdx < 0 {
		startIdx = 0
	}

	result.Items = allMessages[startIdx:endIdx]
	result.HasMoreBefore = startIdx > 0
	result.HasMoreAfter = endIdx < len(allMessages)

	if len(result.Items) > 0 {
		result.OldestCursor = fmt.Sprintf("%d", result.Items[0].Index)
		result.NewestCursor = fmt.Sprintf("%d", result.Items[len(result.Items)-1].Index)
	}

	return result, nil
}

func extractMessage(raw map[string]interface{}, sessionID string, index int) *Message {
	role := "unknown"
	if r, ok := raw["role"].(string); ok {
		role = r
	} else if _, ok := raw["user"].(string); ok {
		role = "user"
	} else if _, ok := raw["assistant"].(string); ok {
		role = "assistant"
	} else if eventType, ok := raw["event_type"].(string); ok {
		if eventType == "command_output" || eventType == "tool_call" {
			role = "tool"
		} else {
			role = eventType
		}
	}

	kind := "message"
	if k, ok := raw["kind"].(string); ok {
		kind = k
	}

	text := ""
	if content, ok := raw["content"].(string); ok {
		text = content
	} else if textVal, ok := raw["text"].(string); ok {
		text = textVal
	} else if output, ok := raw["output"].(string); ok {
		text = output
	} else if userText, ok := raw["user"].(string); ok {
        text = userText
    } else if asstText, ok := raw["assistant"].(string); ok {
        text = asstText
    } else {
		b, _ := json.MarshalIndent(raw, "", "  ")
		text = string(b)
        kind = "raw"
	}

	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	return &Message{
		ID:        fmt.Sprintf("evt_%s_%d", sessionID, index),
		SessionID: sessionID,
		Role:      role,
		Kind:      kind,
		Text:      text,
		Direction: DetectDirection(text),
		Index:     index,
	}
}
