package protocol

import "encoding/json"

type RequestID = string

type InitializeParams struct {
	ClientInfo   ClientInfo   `json:"clientInfo"`
	Capabilities Capabilities `json:"capabilities"`
}

type ClientInfo struct {
	Name    string `json:"name"`
	Title   string `json:"title"`
	Version string `json:"version"`
}

type Capabilities struct {
	ExperimentalAPI bool `json:"experimentalApi"`
}

type ThreadStartParams struct {
	Model                  *string `json:"model,omitempty"`
	CWD                    *string `json:"cwd,omitempty"`
	ServiceTier            *string `json:"serviceTier,omitempty"`
	ApprovalPolicy         *string `json:"approvalPolicy,omitempty"`
	Sandbox                *string `json:"sandbox,omitempty"`
	ExperimentalRawEvents  bool    `json:"experimentalRawEvents"`
	PersistExtendedHistory bool    `json:"persistExtendedHistory"`
}

type ThreadResumeParams struct {
	ThreadID               string  `json:"threadId"`
	Model                  *string `json:"model,omitempty"`
	CWD                    *string `json:"cwd,omitempty"`
	ServiceTier            *string `json:"serviceTier,omitempty"`
	ApprovalPolicy         *string `json:"approvalPolicy,omitempty"`
	Sandbox                *string `json:"sandbox,omitempty"`
	PersistExtendedHistory bool    `json:"persistExtendedHistory"`
}

type ThreadForkParams struct {
	ThreadID               string  `json:"threadId"`
	BeforeTurnID           *string `json:"beforeTurnId,omitempty"`
	Model                  *string `json:"model,omitempty"`
	CWD                    *string `json:"cwd,omitempty"`
	ServiceTier            *string `json:"serviceTier,omitempty"`
	ApprovalPolicy         *string `json:"approvalPolicy,omitempty"`
	Sandbox                *string `json:"sandbox,omitempty"`
	Ephemeral              *bool   `json:"ephemeral,omitempty"`
	PersistExtendedHistory bool    `json:"persistExtendedHistory"`
}

type ThreadSummary struct {
	ID string `json:"id"`
}

type ThreadStartResponse struct {
	Thread          ThreadSummary `json:"thread"`
	Model           string        `json:"model"`
	ReasoningEffort *string       `json:"reasoningEffort"`
}

type ThreadResumeResponse = ThreadStartResponse
type ThreadForkResponse = ThreadStartResponse

type ThreadRevertParams struct {
	ThreadID     string `json:"threadId"`
	BeforeTurnID string `json:"beforeTurnId"`
}

type UserInput struct {
	Type         string   `json:"type"`
	Text         string   `json:"text,omitempty"`
	TextElements []string `json:"text_elements,omitempty"`
	URL          string   `json:"url,omitempty"`
	Path         string   `json:"path,omitempty"`
	Name         string   `json:"name,omitempty"`
}

func (input UserInput) MarshalJSON() ([]byte, error) {
	switch input.Type {
	case "text":
		return json.Marshal(struct {
			Type         string   `json:"type"`
			Text         string   `json:"text"`
			TextElements []string `json:"text_elements"`
		}{Type: input.Type, Text: input.Text, TextElements: nonNilStrings(input.TextElements)})
	case "image":
		return json.Marshal(struct {
			Type string `json:"type"`
			URL  string `json:"url"`
		}{input.Type, input.URL})
	case "localImage":
		return json.Marshal(struct {
			Type string `json:"type"`
			Path string `json:"path"`
		}{input.Type, input.Path})
	case "skill", "mention":
		return json.Marshal(struct {
			Type string `json:"type"`
			Name string `json:"name"`
			Path string `json:"path"`
		}{input.Type, input.Name, input.Path})
	default:
		type alias UserInput
		return json.Marshal(alias(input))
	}
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

type CollaborationMode struct {
	Mode     string                    `json:"mode"`
	Settings CollaborationModeSettings `json:"settings"`
}

type CollaborationModeSettings struct {
	Model                 *string `json:"model"`
	ReasoningEffort       *string `json:"reasoning_effort"`
	DeveloperInstructions *string `json:"developer_instructions"`
}

type TurnStartParams struct {
	ThreadID          string             `json:"threadId"`
	Input             []UserInput        `json:"input"`
	ApprovalPolicy    *string            `json:"approvalPolicy,omitempty"`
	Model             *string            `json:"model,omitempty"`
	Effort            *string            `json:"effort,omitempty"`
	ServiceTier       *string            `json:"serviceTier,omitempty"`
	CollaborationMode *CollaborationMode `json:"collaborationMode,omitempty"`
}

type TurnSummary struct {
	ID     string     `json:"id"`
	Status string     `json:"status"`
	Error  *TurnError `json:"error"`
}

type TurnError struct {
	Message string `json:"message,omitempty"`
}

type TurnStartResponse struct {
	Turn TurnSummary `json:"turn"`
}

type TurnSteerParams struct {
	ThreadID       string      `json:"threadId"`
	Input          []UserInput `json:"input"`
	ExpectedTurnID string      `json:"expectedTurnId"`
}

type TurnInterruptParams struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
}
